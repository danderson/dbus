package dbus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
		t:     t,
		calls: map[uint32]*pendingCall{},
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

	return ret, nil
}

// Conn is a DBus connection.
type Conn struct {
	t        transport.Transport
	writeMu  sync.Mutex
	clientID string

	bus Interface

	mu         sync.Mutex
	closed     bool
	calls      map[uint32]*pendingCall
	lastSerial uint32
	watchers   mapset.Set[*Watcher]
	claims     mapset.Set[*Claim]
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
		defer c.mu.Unlock()
		c.closed = true
		pend, c.calls = c.calls, nil
		ws, c.watchers = c.watchers, nil
		cs, c.claims = c.claims, nil
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

func (c *Conn) writeMsg(ctx context.Context, files []*os.File, hdr *header, body []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	enc := fragments.Encoder{
		Order:  fragments.NativeEndian,
		Mapper: encoderFor,
	}
	if err := enc.Value(ctx, hdr); err != nil {
		return err
	}
	enc.Write(body)
	if _, err := c.t.WriteWithFiles(enc.Out, files); err != nil {
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

	ctx := context.Background()
	if len(msg.files) > 0 {
		ctx = withContextFiles(ctx, msg.files)
	}
	if msg.Sender != "" && msg.Path != "" && msg.Interface != "" {
		ctx = withContextSender(ctx, c.Peer(msg.Sender).Object(msg.Path).Interface(msg.Interface))
	}

	switch msg.Type {
	case msgTypeCall:
		log.Printf("TODO: CALL")
	case msgTypeReturn:
		return c.dispatchReturn(ctx, msg)
	case msgTypeError:
		return c.dispatchErr(msg)
	case msgTypeSignal:
		return c.dispatchSignal(ctx, msg)
	}
	return nil
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

	ctx = withContextSender(ctx, pending.iface)

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
	signalType := signalNameToType[signalKey{msg.Interface, msg.Member}]
	if signalType == nil {
		signalType = msg.Signature.Type()
	}

	sender, _ := ContextSender(ctx)

	var signal reflect.Value
	if signalType != nil {
		signal = reflect.New(signalType)
		if err := msg.body.Value(ctx, signal.Interface()); err != nil {
			return err
		}
	} else {
		signal = reflect.ValueOf(&struct{}{})
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for w := range c.watchers {
		w.deliver(sender, &msg.header, signal)
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

	var (
		payload []byte
		sig     Signature
		files   []*os.File
		err     error
	)
	if body != nil {
		ctx := withContextPutFiles(context.Background(), &files)
		payload, err = marshal(ctx, body, fragments.NativeEndian)
		if err != nil {
			return err
		}

		sig, err = SignatureOf(body)
		if err != nil {
			return err
		}
		sig = sig.asMsgBody()
	}

	hdr := header{
		Type:        msgTypeCall,
		Flags:       0,
		Version:     1,
		Length:      uint32(len(payload)),
		Serial:      serial,
		Destination: destination,
		Path:        path,
		Interface:   iface,
		Member:      method,
		NumFDs:      uint32(len(files)),
	}
	if body != nil {
		hdr.Signature = sig
	}
	for _, f := range opts {
		hdr.Flags |= f.callOptionValue()
	}
	if err := hdr.Valid(); err != nil {
		return err
	}

	if err := c.writeMsg(context.Background(), files, &hdr, payload); err != nil {
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
