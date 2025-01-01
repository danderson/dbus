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

	go ret.readLoop()

	req := Request{
		Destination: "org.freedesktop.DBus",
		Path:        "/org/freedesktop/DBus",
		Interface:   "org.freedesktop.DBus",
		Method:      "Hello",
	}
	if err := ret.Call(ctx, req, &ret.clientID); err != nil {
		ret.Close()
		return nil, fmt.Errorf("getting DBus client ID: %w", err)
	}

	return ret, nil
}

// Conn is a DBus connection.
type Conn struct {
	t        transport.Transport
	clientID string

	mu         sync.Mutex
	calls      map[uint32]*pendingCall
	lastSerial uint32
}

type pendingCall struct {
	notify chan struct{}
	resp   any
	err    error
}

// Close closes the DBus connection. Any in-flight requests are
// canceled, both outbound and inbound.
func (c *Conn) Close() error {
	return c.t.Close()
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
	if err := Unmarshal(c.t, fragments.NativeEndian, &hdr); err != nil {
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

	switch hdr.Type {
	case msgTypeCall:
		log.Printf("TODO: CALL")
	case msgTypeReturn:
		return c.dispatchReturn(&hdr, bodyReader, fs)
	case msgTypeError:
		return c.dispatchErr(&hdr, bodyReader)
	case msgTypeSignal:
		log.Printf("TODO: SIGNAL %v", hdr)
	}
	return nil
}

func (c *Conn) dispatchReturn(hdr *header, body io.Reader, _ []*os.File) error {
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

	if pending.resp != nil {
		if err := Unmarshal(body, hdr.Order.Order(), pending.resp); err != nil {
			return err
		}
	}
	close(pending.notify)
	return nil
}

func (c *Conn) dispatchErr(hdr *header, body io.Reader) error {
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

// Call calls a remote method over the bus and records the response in
// the provided pointer.
//
// It is the caller's responsibility to supply the correct types of
// request.Body and response for the method being called.
func (c *Conn) Call(ctx context.Context, request Request, response any) error {
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
		body []byte
		sig  Signature
		err  error
	)
	if request.Body != nil {
		body, err = Marshal(request.Body, fragments.NativeEndian)
		if err != nil {
			return err
		}

		sig, err = SignatureOf(request.Body)
		if err != nil {
			return err
		}
	}

	hdr := header{
		Type:        msgTypeCall,
		Flags:       0,
		Version:     1,
		Length:      uint32(len(body)),
		Serial:      serial,
		Path:        request.Path,
		Interface:   request.Interface,
		Member:      request.Method,
		Destination: request.Destination,
	}
	if request.Body != nil {
		hdr.Signature = sig
	}
	if request.OneWay {
		hdr.Flags |= 0x1
	}
	if request.NoAutoStart {
		hdr.Flags |= 0x2
	}
	if request.AllowInteraction {
		hdr.Flags |= 0x4
	}
	if err := hdr.Valid(); err != nil {
		return err
	}

	bs, err := Marshal(&hdr, fragments.NativeEndian)
	if err != nil {
		return err
	}
	if _, err := c.t.Write(bs); err != nil {
		return err // TODO: close transport?
	}
	if request.Body != nil {
		if _, err := c.t.Write(body); err != nil {
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

// Request is a DBus method call request.
type Request struct {
	// Destination is the DBus peer to which the request should be
	// sent.
	Destination string
	// Path is the path to the target object published by the peer.
	Path ObjectPath
	// Interface is the name of the interface containing the method to
	// invoke.
	Interface string
	// Method is the method to call on the interface.
	Method string

	// OneWay informs the peer that the call is one-way, with no
	// response desired. The [Conn.Call] will complete as soon as the
	// request has been sent, and there is no way to tell whether the
	// peer received it or processed it successfully.
	OneWay bool
	// AllowInteraction, if true, indicates that you are willing to
	// wait for an interactive authorization prompt, if the system
	// policy requires it. Requests that allow interaction should
	// expect a long delay (at least several seconds) to get a
	// response. Making a non-interactive request to a privileged
	// endpoint will promptly return a permission error.
	AllowInteraction bool
	// NoAutoStart tells the bus that it must not automatically start
	// services as a result of this request. Requests to a peer that
	// would require on-demand starting will return an error.
	NoAutoStart bool

	// Body is the request body. The body's type signature must match
	// the method being invoked. If the method accepts multiple
	// parameters, Body must be a struct whose fields are the method
	// parameters. Body can be nil for methods that take no input
	// parameters.
	Body any
}
