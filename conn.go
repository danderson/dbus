package dbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"

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
	clientID string

	bus Interface

	mu         sync.Mutex
	calls      map[uint32]*pendingCall
	lastSerial uint32
}

type pendingCall struct {
	notify chan struct{}
	iface  Interface
	resp   any
	err    error
}

// Close closes the DBus connection. Any in-flight requests are
// canceled, both outbound and inbound.
func (c *Conn) Close() error {
	return c.t.Close()
}

func (c *Conn) Peer(name string) Peer {
	return Peer{
		c:    c,
		name: name,
	}
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

func (c *Conn) dispatchMsg() error {
	var hdr header
	if err := Unmarshal(context.Background(), c.t, fragments.NativeEndian, &hdr); err != nil {
		return err
	}
	bodyReader := io.LimitReader(c.t, int64(hdr.Length))
	defer func() {
		io.Copy(io.Discard, bodyReader)
	}()
	fs, err := c.t.GetFiles(int(hdr.NumFDs))
	if err != nil {
		return err
	}

	if err := hdr.Valid(); err != nil {
		return fmt.Errorf("received invalid header: %w", err)
	}

	ctx := context.Background()
	if len(fs) > 0 {
		ctx = withContextFiles(ctx, fs)
	}
	if hdr.Sender != "" && hdr.Path != "" && hdr.Interface != "" {
		ctx = withContextSender(ctx, c.Peer(hdr.Sender).Object(hdr.Path).Interface(hdr.Interface))
	}

	switch hdr.Type {
	case msgTypeCall:
		log.Printf("TODO: CALL")
	case msgTypeReturn:
		return c.dispatchReturn(ctx, &hdr, bodyReader, fs)
	case msgTypeError:
		return c.dispatchErr(ctx, &hdr, bodyReader)
	case msgTypeSignal:
		return c.dispatchSignal(ctx, &hdr, bodyReader)
	}
	return nil
}

func (c *Conn) dispatchReturn(ctx context.Context, hdr *header, body io.Reader, _ []*os.File) error {
	// TODO: correct pairing of files and body
	pending := func() *pendingCall {
		c.mu.Lock()
		defer c.mu.Unlock()
		ret := c.calls[hdr.ReplySerial]
		delete(c.calls, hdr.ReplySerial)
		return ret
	}()

	if pending == nil {
		// Response to a canceled call
		return nil
	}

	ctx = withContextSender(ctx, pending.iface)

	if pending.resp != nil {
		if err := Unmarshal(ctx, body, hdr.Order.Order(), pending.resp); err != nil {
			return err
		}
	}
	close(pending.notify)
	return nil
}

func (c *Conn) dispatchErr(ctx context.Context, hdr *header, body io.Reader) error {
	pending := func() *pendingCall {
		c.mu.Lock()
		defer c.mu.Unlock()
		ret := c.calls[hdr.ReplySerial]
		delete(c.calls, hdr.ReplySerial)
		return ret
	}()

	if pending == nil {
		// Response to a canceled call
		return nil
	}

	errStr := func() string {
		if hdr.Signature.IsZero() {
			return ""
		}
		for p := range hdr.Signature.Parts() {
			if p.Type() != reflect.TypeFor[string]() {
				return ""
			}
			break
		}
		dec := fragments.Decoder{
			Order: hdr.Order.Order(),
			In:    body,
		}
		errStr, err := dec.String()
		if err != nil {
			return fmt.Sprintf("got error while decoding error detail: %v", err)
		}
		return errStr
	}()

	pending.err = CallError{
		Name:   hdr.ErrName,
		Detail: errStr,
	}
	close(pending.notify)
	return nil
}

func (c *Conn) dispatchSignal(ctx context.Context, hdr *header, body io.Reader) error {
	signalType := typeForSignal(hdr.Interface, hdr.Member, hdr.Signature)

	sender, _ := ContextSender(ctx)

	var signal any
	if signalType != nil {
		signal = reflect.New(signalType).Interface()
		if err := Unmarshal(ctx, body, hdr.Order.Order(), signal); err != nil {
			return err
		}
	}

	log.Printf("SIGNAL: %v %s %T %#v", sender, hdr.Member, signal, signal)

	return nil
}

type CallOption interface {
	callOptionValue() byte
}

type callOption byte

func (o callOption) callOptionValue() byte {
	return byte(o)
}

func NoReply() CallOption {
	return callOption(1)
}

func NoAutoStart() CallOption {
	return callOption(2)
}

func AllowInteraction() CallOption {
	return callOption(4)
}

// Call calls a remote method over the bus and records the response in
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
		c.lastSerial++
		pend := &pendingCall{
			notify: make(chan struct{}, 1),
			resp:   response,
		}
		c.calls[c.lastSerial] = pend
		return c.lastSerial, pend
	}()
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
		err     error
	)
	if body != nil {
		payload, err = Marshal(context.Background(), body, fragments.NativeEndian)
		if err != nil {
			return err
		}

		sig, err = SignatureOf(body)
		if err != nil {
			return err
		}
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

	bs, err := Marshal(context.Background(), &hdr, fragments.NativeEndian)
	if err != nil {
		return err
	}
	if _, err := c.t.Write(bs); err != nil {
		return err // TODO: close transport?
	}
	if body != nil {
		if _, err := c.t.Write(payload); err != nil {
			return err // TODO: close transport?
		}
	}

	select {
	case <-pending.notify:
		return pending.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
