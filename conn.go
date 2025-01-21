package dbus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/creachadair/mds/mapset"
	"github.com/danderson/dbus/fragments"
	"github.com/danderson/dbus/transport"
)

// SystemBus connects to the system bus.
func SystemBus(ctx context.Context) (*Conn, error) {
	return newConn(ctx, "/run/dbus/system_bus_socket")
}

// SessionBus connects to the current user's session bus.
func SessionBus(ctx context.Context) (*Conn, error) {
	path := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if path == "" {
		return nil, errors.New("session bus not available")
	}
	for _, uri := range strings.Split(path, ";") {
		addr, ok := strings.CutPrefix(uri, "unix:path=")
		if !ok {
			continue
		}
		return newConn(ctx, addr)
	}
	return nil, fmt.Errorf("could not find usable session bus address in DBUS_SESSION_BUS_ADDRESS value %q", path)
}

func newConn(ctx context.Context, path string) (*Conn, error) {
	t, err := transport.DialUnix(ctx, path)
	if err != nil {
		return nil, err
	}
	ret := &Conn{
		t: t,
		enc: fragments.Encoder{
			Order:  fragments.NativeEndian,
			Mapper: encoderFor,
		},
		calls:    map[uint32]*pendingCall{},
		handlers: map[interfaceMethod]handlerFunc{},
	}
	ret.bus = ret.
		Peer("org.freedesktop.DBus").
		Object("/org/freedesktop/DBus").
		Interface("org.freedesktop.DBus")

	go ret.readLoop()

	if err := ret.bus.Call(ctx, "Hello", nil, &ret.clientID); err != nil {
		ret.Close()
		return nil, fmt.Errorf("getting DBus client ID: %w", err)
	}

	// Implement the Peer interface, on all objects.
	ret.Handle("org.freedesktop.DBus.Peer", "Ping", func(context.Context, ObjectPath) error {
		return nil
	})
	uuid := sync.OnceValues(func() (string, error) {
		bs, err := os.ReadFile("/etc/machine-id")
		if errors.Is(err, fs.ErrNotExist) {
			bs, err = os.ReadFile("/var/lib/dbus/machine-id")
		}
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(bs)), nil
	})
	ret.Handle("org.freedesktop.DBus.Peer", "GetMachineId", func(context.Context, ObjectPath) (string, error) {
		return uuid()
	})

	return ret, nil
}

// Conn is a DBus connection.
type Conn struct {
	t        transport.Transport
	clientID string

	bus Interface

	writeMu sync.Mutex
	enc     fragments.Encoder
	encBody []byte
	encHdr  []byte

	mu         sync.Mutex
	closed     bool
	calls      map[uint32]*pendingCall
	lastSerial uint32
	watchers   mapset.Set[*Watcher]
	claims     mapset.Set[*Claim]
	handlers   map[interfaceMethod]handlerFunc
}

type interfaceMethod struct {
	Interface string
	Method    string
}

type pendingCall struct {
	notify chan struct{}
	iface  Interface
	resp   any
	err    error
}

// Close closes the DBus connection.
func (c *Conn) Close() error {
	var (
		pend map[uint32]*pendingCall
		ws   mapset.Set[*Watcher]
		cs   mapset.Set[*Claim]
	)
	{
		c.mu.Lock()
		c.closed = true
		pend, c.calls = c.calls, nil
		ws, c.watchers = c.watchers, nil
		cs, c.claims = c.claims, nil
		c.mu.Unlock()
	}
	for c := range maps.Values(pend) {
		c.err = net.ErrClosed
		close(c.notify)
	}
	for w := range ws {
		w.Close()
	}
	for c := range cs {
		c.Close()
	}
	return c.t.Close()
}

// LocalName returns the connection's unique bus name.
func (c *Conn) LocalName() string {
	return c.clientID
}

// Peer returns a Peer for the given bus name.
//
// The returned value is a purely local handle. It does not indicate
// that the requested peer exists, or that it is currently reachable.
func (c *Conn) Peer(name string) Peer {
	return Peer{
		c:    c,
		name: name,
	}
}

func (c *Conn) writeMsg(ctx context.Context, hdr *header, body any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var files []*os.File
	c.encBody = c.encBody[:0]
	if body != nil {
		bodyCtx := withContextHeader(ctx, c, hdr)
		bodyCtx = withContextFiles(bodyCtx, &files)
		c.enc.Out = c.encBody
		if err := c.enc.Value(ctx, body); err != nil {
			return err
		}
		sig, err := SignatureOf(body)
		if err != nil {
			return err
		}
		hdr.Length = uint32(len(c.enc.Out))
		hdr.Signature = sig.asMsgBody()
		hdr.NumFDs = uint32(len(files))
		c.encBody = c.enc.Out
	}

	c.enc.Out = c.encHdr[:0]
	if err := c.enc.Value(ctx, hdr); err != nil {
		return err
	}
	c.encHdr = c.enc.Out

	if _, err := c.t.WriteWithFiles(c.encHdr, files); err != nil {
		return err
	}
	if _, err := c.t.Write(c.encBody); err != nil {
		return err
	}

	return nil
}

func (c *Conn) readLoop() {
	for {
		if err := c.dispatchMsg(); errors.Is(err, net.ErrClosed) {
			// Conn was shut down.
			return
		} else if err != nil {
			// Errors that bubble out here represent a failure to
			// conform to the DBus protocol, and is fatal to the
			// Conn.
			log.Printf("read error: %v", err)
		}
	}
}

type msg struct {
	header
	body  fragments.Decoder
	files []*os.File
}

// readMsg reads one complete DBus message from c.t. Must not be
// called concurrently (Conn.dispatchMsg ensures this).
func (c *Conn) readMsg() (*msg, error) {
	ret := &msg{
		body: fragments.Decoder{
			Order:  fragments.NativeEndian,
			Mapper: decoderFor,
			In:     c.t,
		},
	}
	if err := ret.body.Value(context.Background(), &ret.header); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(c.t, int64(ret.header.Length)))
	if err != nil {
		return nil, err
	}
	ret.body.In = bytes.NewBuffer(body)
	ret.files, err = c.t.GetFiles(int(ret.header.NumFDs))
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (c *Conn) dispatchMsg() error {
	msg, err := c.readMsg()
	if err != nil {
		return err
	}
	if err := msg.Valid(); err != nil {
		return fmt.Errorf("received invalid header: %w", err)
	}

	ctx := withContextHeader(context.Background(), c, &msg.header)
	if len(msg.files) > 0 {
		ctx = withContextFiles(ctx, &msg.files)
	}

	switch msg.Type {
	case msgTypeCall:
		go c.dispatchCall(ctx, msg)
	case msgTypeReturn:
		return c.dispatchReturn(ctx, msg)
	case msgTypeError:
		return c.dispatchErr(msg)
	case msgTypeSignal:
		return c.dispatchSignal(ctx, msg)
	}
	return nil
}

func (c *Conn) dispatchCall(ctx context.Context, msg *msg) {
	handler, serial := func() (handlerFunc, uint32) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return nil, 0
		}
		handler := c.handlers[interfaceMethod{msg.Interface, msg.Member}]
		c.lastSerial++
		return handler, c.lastSerial
	}()

	respHdr := &header{
		Type:        msgTypeReturn,
		Version:     1,
		Serial:      serial,
		Destination: msg.Sender,
		ReplySerial: msg.Serial,
	}
	if handler == nil {
		respHdr.Type = msgTypeError
		respHdr.ErrName = "org.freedesktop.DBus.Error.Failed"
		c.writeMsg(ctx, respHdr, "no such method")
		return
	}

	resp, err := handler(ctx, msg.Path, &msg.body)
	if err != nil {
		respHdr.Type = msgTypeError
		respHdr.ErrName = "org.freedesktop.DBus.Error.Failed"
		c.writeMsg(ctx, respHdr, err.Error())
		return
	}
	c.writeMsg(ctx, respHdr, resp)
}

func (c *Conn) dispatchReturn(ctx context.Context, msg *msg) error {
	pending := func() *pendingCall {
		c.mu.Lock()
		defer c.mu.Unlock()
		ret := c.calls[msg.ReplySerial]
		delete(c.calls, msg.ReplySerial)
		return ret
	}()

	if pending == nil {
		// Response to a canceled call
		return nil
	}

	if pending.resp != nil {
		if err := msg.body.Value(ctx, pending.resp); err != nil {
			return err
		}
	}
	close(pending.notify)
	return nil
}

func (c *Conn) dispatchErr(msg *msg) error {
	pending := func() *pendingCall {
		c.mu.Lock()
		defer c.mu.Unlock()
		ret := c.calls[msg.ReplySerial]
		delete(c.calls, msg.ReplySerial)
		return ret
	}()

	if pending == nil {
		// Response to a canceled call
		return nil
	}

	errStr := func() string {
		if msg.Signature.IsZero() {
			return ""
		}
		if s := msg.Signature.String(); s != "s" && !strings.HasPrefix(s, "(s") {
			return ""
		}
		errStr, err := msg.body.String()
		if err != nil {
			return fmt.Sprintf("got error while decoding error detail: %v", err)
		}
		return errStr
	}()

	pending.err = CallError{
		Name:   msg.ErrName,
		Detail: errStr,
	}
	close(pending.notify)
	return nil
}

func (c *Conn) dispatchSignal(ctx context.Context, msg *msg) error {
	signalType := signalTypeFor(msg.Interface, msg.Member)
	if signalType == nil {
		signalType = msg.Signature.asStruct().Type()
	}
	if signalType == nil {
		signalType = reflect.TypeFor[struct{}]()
	}

	emitter, _ := ContextEmitter(ctx)

	signal := reflect.New(signalType)
	if err := msg.body.Value(ctx, signal.Interface()); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for w := range c.watchers {
		w.deliver(emitter, &msg.header, signal)
	}

	return nil
}

// CallOption is a generic option for a DBus method call.
type CallOption interface {
	callOptionValue() byte
}

type callOption byte

func (o callOption) callOptionValue() byte {
	return byte(o)
}

// NoReply indicates that the method call is one-way, and that the
// recipient must not generate a response message.
func NoReply() CallOption {
	return callOption(1)
}

// NoAutoStart indicates that the recipient should not be autostarted
// if it's not already running.
func NoAutoStart() CallOption {
	return callOption(2)
}

// AllowInteraction indicates that the caller is willing to wait an
// extended amount of time for the method call to be interactively
// authorized by the user.
func AllowInteraction() CallOption {
	return callOption(4)
}

// call calls a remote method over the bus and records the response in
// the provided pointer.
//
// It is the caller's responsibility to supply the correct types of
// request.Body and response for the method being called.
func (c *Conn) call(ctx context.Context, destination string, path ObjectPath, iface, method string, body any, response any, opts ...CallOption) error {
	if response != nil && reflect.TypeOf(response).Kind() != reflect.Pointer {
		return errors.New("response parameter in Call must be a pointer, or nil")
	}

	serial, pending := func() (uint32, *pendingCall) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return 0, nil
		}

		c.lastSerial++
		pend := &pendingCall{
			notify: make(chan struct{}, 1),
			resp:   response,
		}
		c.calls[c.lastSerial] = pend
		return c.lastSerial, pend
	}()
	if pending == nil {
		return net.ErrClosed
	}
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.calls[serial] == pending {
			delete(c.calls, serial)
		}
	}()

	hdr := header{
		Type:        msgTypeCall,
		Flags:       0,
		Version:     1,
		Serial:      serial,
		Destination: destination,
		Path:        path,
		Interface:   iface,
		Member:      method,
	}
	for _, f := range opts {
		hdr.Flags |= f.callOptionValue()
	}
	if err := hdr.Valid(); err != nil {
		return err
	}

	if err := c.writeMsg(context.Background(), &hdr, body); err != nil {
		return err // TODO: close transport?
	}

	if !hdr.WantReply() {
		return nil
	}

	select {
	case <-pending.notify:
		return pending.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// EmitSignal broadcasts signal from obj.
//
// The signal's type must be registered in advance with
// [RegisterSignalType].
func (c *Conn) EmitSignal(ctx context.Context, obj ObjectPath, signal any) error {
	t := reflect.TypeOf(signal)
	k, ok := signalNameFor(t)
	if !ok {
		return fmt.Errorf("unknown signal type %s", t)
	}
	serial := func() uint32 {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return 0
		}
		c.lastSerial++
		return c.lastSerial
	}()
	hdr := header{
		Type:      msgTypeSignal,
		Version:   1,
		Serial:    serial,
		Path:      obj,
		Interface: k.Interface,
		Member:    k.Method,
	}
	return c.writeMsg(ctx, &hdr, signal)
}

// Handle calls fn to handle incoming method calls to methodName on
// interfaceName.
//
// fn must have one of the following type signatures, where ReqType
// and RetType are any types compatible with DBus:
//
//	func(context.Context, dbus.ObjectPath) error
//	func(context.Context, dbus.ObjectPath) (RetType, error)
//	func(context.Context, dbus.ObjectPath, ReqType) error
//	func(context.Context, dbus.ObjectPath, ReqType) (RetType, error)
//
// Handle panics if fn is not one of the above type signatures.
func (c *Conn) Handle(interfaceName, methodName string, fn any) {
	handler := handlerForFunc(fn)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[interfaceMethod{interfaceName, methodName}] = handler
}

type handlerFunc func(ctx context.Context, object ObjectPath, req *fragments.Decoder) (any, error)

func handlerForFunc(fn any) handlerFunc {
	v := reflect.ValueOf(fn)
	if !v.IsValid() {
		panic(errors.New("nil handler function given to Handle"))
	}
	t := v.Type()
	if t.Kind() != reflect.Func {
		panic(fmt.Errorf("Handle called with non-function handler type %s", t))
	}
	ni, no := t.NumIn(), t.NumOut()

	const msgInvalidHandlerSignature = "invalid signature %s for handler func, valid signatures are:\n  func(context.Context, dbus.ObjectPath, ReqT) (RespT, error)\n  func(context.Context, dbus.ObjectPath) (RespT, error)\n  func(context.Context, dbus.ObjectPath, ReqT) error\n  func(context.Context, dbus.ObjectPath) error"

	if ni < 2 || ni > 3 || no < 1 || no > 2 {
		panic(fmt.Errorf(msgInvalidHandlerSignature, t))
	}
	if !t.In(0).Implements(reflect.TypeFor[context.Context]()) {
		panic(fmt.Errorf(msgInvalidHandlerSignature, t))
	}
	if t.In(1) != reflect.TypeFor[ObjectPath]() {
		panic(fmt.Errorf(msgInvalidHandlerSignature, t))
	}
	if !t.Out(no - 1).Implements(reflect.TypeFor[error]()) {
		panic(fmt.Errorf(msgInvalidHandlerSignature, t))
	}
	var (
		reqDec fragments.DecoderFunc
		err    error
	)
	if ni == 3 {
		reqDec, err = decoderFor(t.In(2))
		if err != nil {
			panic(fmt.Errorf("request type %s is not a valid DBus type: %w", t.In(1), err))
		}
	}
	if no == 2 {
		if _, err = encoderFor(t.Out(0)); err != nil {
			if err != nil {
				panic(fmt.Errorf("response type %s is not a valid DBus type: %w", t.Out(0), err))
			}
		}
	}

	type s struct{ numIn, numOut int }
	switch (s{ni, no}) {
	case s{2, 1}:
		handler := fn.(func(context.Context, ObjectPath) error)
		return func(ctx context.Context, obj ObjectPath, req *fragments.Decoder) (any, error) {
			return nil, handler(ctx, obj)
		}
	case s{2, 2}:
		return func(ctx context.Context, obj ObjectPath, req *fragments.Decoder) (any, error) {
			rets := v.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(obj)})
			if err, ok := rets[1].Interface().(error); ok && err != nil {
				return nil, err
			}
			return rets[0].Interface(), nil
		}
	case s{3, 1}:
		return func(ctx context.Context, obj ObjectPath, req *fragments.Decoder) (any, error) {
			body := reflect.New(t.In(1))
			if err := reqDec(ctx, req, body); err != nil {
				return nil, err
			}
			rets := v.Call([]reflect.Value{
				reflect.ValueOf(ctx),
				reflect.ValueOf(obj),
				body.Elem(),
			})
			if err, ok := rets[0].Interface().(error); ok && err != nil {
				return nil, err
			}
			return rets[1].Interface(), nil
		}
	case s{3, 2}:
		return func(ctx context.Context, obj ObjectPath, req *fragments.Decoder) (any, error) {
			body := reflect.New(t.In(1))
			if err := reqDec(ctx, req, body); err != nil {
				return nil, err
			}
			rets := v.Call([]reflect.Value{
				reflect.ValueOf(ctx),
				reflect.ValueOf(obj),
				body.Elem(),
			})
			if err, ok := rets[1].Interface().(error); ok && err != nil {
				return nil, err
			}
			return rets[0].Interface(), nil
		}
	default:
		panic("unreachable")
	}
}
