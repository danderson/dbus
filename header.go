package dbus

import (
	"fmt"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

type byteOrder bool

func (*byteOrder) AlignDBus() int { return 1 }

var byteOrderSignature = mkSignature(reflect.TypeFor[uint8]())

func (*byteOrder) SignatureDBus() Signature { return byteOrderSignature }

func (*byteOrder) MarshalDBus(e *fragments.Encoder) error {
	e.ByteOrderFlag()
	return nil
}
func (b *byteOrder) UnmarshalDBus(d *fragments.Decoder) error {
	d.ByteOrderFlag()
	*b = d.Order == fragments.BigEndian
	return nil
}

func (b *byteOrder) Order() fragments.ByteOrder {
	if *b {
		return fragments.BigEndian
	} else {
		return fragments.LittleEndian
	}
}

type msgType byte

const (
	msgTypeCall msgType = iota + 1
	msgTypeReturn
	msgTypeError
	msgTypeSignal
)

type structAlign struct{}

func (*structAlign) AlignDBus() int           { return 8 }
func (*structAlign) SignatureDBus() Signature { return Signature{} }

func (*structAlign) MarshalDBus(*fragments.Encoder) error   { return nil }
func (*structAlign) UnmarshalDBus(*fragments.Decoder) error { return nil }

// headerExtra is the additional headers that come after the
// fixed-length portion of the header. The set of required and
// optional fields depends on the message type.
type headerExtra struct {
	// Path is the target object for a call, or the source object
	// for a signal. Required for msgTypeCall and msgTypeSignal.
	Path ObjectPath `dbus:"key=1"`
	// Interface is the interface to target for a call, or the
	// source interface for a signal. Required for msgTypeCall and
	// msgTypeSignal.
	Interface string `dbus:"key=2"`
	// Member is the method name for a call, or signal name for a
	// signal. Required for msgTypeCall and msgTypeSignal.
	Member string `dbus:"key=3"`
	// ErrName is the name of the error that occurred. Required
	// for msgTypeError.
	ErrName string `dbus:"key=4"`
	// ReplySerial is the message serial to which this message is
	// replying. Required for msgTypeReturn and msgTypeError.
	ReplySerial uint32 `dbus:"key=5"`
	// Destination is the target for a message. Required for TODO.
	Destination string `dbus:"key=6"`
	// Sender is the client ID of the message sender. The message
	// bus populates this value itself, any sent value is ignored
	// and removed.
	Sender string `dbus:"key=7"`
	// Signature is the type signature of the request
	// body. Required if a message body is present.
	Signature Signature `dbus:"key=8"`
	// NumFDs is the number of file descriptors attached to this
	// message. Required if file descriptors are attached to the
	// message.
	NumFDs uint32 `dbus:"key=9"`
	// Unknown collects remaining unknown extension headers
	// present in the message.
	Unknown map[uint8]Variant
}

// header is a DBus message header, minus the initial byte order
// indicator byte.
type header struct {
	Order byteOrder
	// Type is the message's type.
	Type msgType
	// Flags is the message's flag byte.
	Flags byte
	// Version is the DBus protocol version
	Version uint8
	// Length is the length of the message body, not including the
	// header or padding between header and body.
	Length uint32
	// Serial is the serial for this message. It must be non-zero.
	Serial uint32

	Extra headerExtra
	Align structAlign
}

// Valid checks that the message header is valid for its message type.
func (h *header) Valid() error {
	if h.Serial == 0 {
		return fmt.Errorf("invalid message with zero Serial")
	}
	switch h.Type {
	case 0:
		return fmt.Errorf("invalid message with Type 0")
	case msgTypeCall:
		if h.Extra.Path == "" {
			return fmt.Errorf("missing required header field Path")
		}
		if h.Extra.Interface == "" {
			return fmt.Errorf("missing required header field Interface")
		}
		if h.Extra.Member == "" {
			return fmt.Errorf("missing required header field Member")
		}
		if h.Extra.Destination == "" {
			return fmt.Errorf("missing required header field Destination")
		}
	case msgTypeReturn:
		if h.Extra.ReplySerial == 0 {
			return fmt.Errorf("missing required header field ReplySerial")
		}
	case msgTypeError:
		if h.Extra.ReplySerial == 0 {
			return fmt.Errorf("missing required header field ReplySerial")
		}
		if h.Extra.ErrName == "" {
			return fmt.Errorf("missing required header field ErrName")
		}
	case msgTypeSignal:
		if h.Extra.Path == "" {
			return fmt.Errorf("missing required header field Path")
		}
		if h.Extra.Interface == "" {
			return fmt.Errorf("missing required header field Interface")
		}
		if h.Extra.Member == "" {
			return fmt.Errorf("missing required header field Member")
		}
	default:
		// Unknown message types are suspect, but the spec requires us to
		// gracefully allow them.
	}
	return nil
}

// WantReply reports whether this message requires a response.
func (h *header) WantReply() bool {
	return h.Type == msgTypeCall && h.Flags&0x1 == 0
}

// CanInteract reports whether the message's sender is prepared to
// wait for an interactive authorization prompt, if the sender lacks
// the necessary privileges for the message, and the bus or
// destination wish to trigger an interactive prompt.
func (h header) CanInteract() bool {
	return h.Type == msgTypeCall && h.Flags&0x4 != 0
}