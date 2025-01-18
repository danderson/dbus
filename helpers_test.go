package dbus

import (
	"context"
	"fmt"

	"github.com/danderson/dbus/fragments"
)

// Simple is a struct with simple fields.
type Simple struct {
	A int16
	B bool
}

// Nested is a struct with a struct field.
type Nested struct {
	A byte
	B Simple
}

// Embedded is a struct that embeds another struct by value.
type Embedded struct {
	Simple
	C byte
}

// EmbeddedShadow is a struct that embeds another struct by value,
// with one of the embedded fields shadowed by an outer field.
type EmbeddedShadow struct {
	Simple
	B byte
}

// Arrays is a struct with various degrees of complicated arrays
// inside.
type Arrays struct {
	A []string
	B []Simple
	C [][]Nested
}

// Tree is a self-referential struct that can't be represented in the
// DBus wire format.
type Tree struct {
	Left  *Tree
	Right *Tree
}

// NestedSelfMashalerVal is a struct with a field that implements
// Marshaler/Unmarshaler using value method
// receivers. NestedSelfMashalerVal cannot be unmarshaled, because
// UnmarshalDBus must be implemented on a pointer receiver.
type NestedSelfMashalerVal struct {
	A byte
	B SelfMarshalerVal
}

// NestedSelfMarshalerPtr is a struct with a struct field that
// implements Marshaler/Unmarshaler with pointer method
// receivers.
type NestedSelfMarshalerPtr struct {
	A byte
	B SelfMarshalerPtr
}

// NestedSelfMarshalerPtrPtr is a struct with a struct pointer field
// that implements Marshaler/Unmarshaler with pointer method
// receivers.
type NestedSelfMarshalerPtrPtr struct {
	A byte
	B *SelfMarshalerPtr
}

// Embedded_P is a struct that embeds another struct by pointer.
type Embedded_P struct {
	*Simple
	C byte
}

// Embedded_PV is a struct with 2 layers of embedding, first by value
// then by pointers.
type Embedded_PV struct {
	Embedded_P
}

// Embedded_PVP is a struct that fights other structs online. And also
// a struct with 3 layers of embedding, pointer then value then
// pointer.
type Embedded_PVP struct {
	*Embedded_PV
	D byte
}

// SelfMarshalerVal is a struct that implements Marshaler and
// Unmarshaler, with value method receivers. Note the
// Unmarshaler implementation is deliberately unusable
// (UnmarshalDBus must have a pointer receiver).
type SelfMarshalerVal struct {
	B byte
}

func (s SelfMarshalerVal) MarshalDBus(ctx context.Context, e *fragments.Encoder) error {
	e.Pad(3)
	e.Write([]byte{0, s.B + 1})
	return nil
}

func (s SelfMarshalerVal) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	if err := d.Pad(3); err != nil {
		return err
	}
	bs, err := d.Read(2)
	if err != nil {
		return err
	}
	if bs[0] != 0 {
		return fmt.Errorf("unexpected non-zero first bytes %x", bs[0])
	}
	s.B = bs[1] - 1
	return nil
}

func (s SelfMarshalerVal) IsDBusStruct() bool { return false }

func (s SelfMarshalerVal) SignatureDBus() Signature {
	return mustSignatureFor[uint16]()
}

// SelfMarshalerPtr is a struct that implements Marshaler and
// Unmarshaler with pointer method receivers.
type SelfMarshalerPtr struct {
	B byte
}

func (s *SelfMarshalerPtr) MarshalDBus(ctx context.Context, e *fragments.Encoder) error {
	e.Pad(3)
	e.Write([]byte{0, s.B + 1})
	return nil
}

func (s *SelfMarshalerPtr) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	if err := d.Pad(3); err != nil {
		return err
	}
	bs, err := d.Read(2)
	if err != nil {
		return err
	}
	if bs[0] != 0 {
		return fmt.Errorf("unexpected non-zero first bytes %x", bs[0])
	}
	s.B = bs[1] - 1
	return nil
}

func (s *SelfMarshalerPtr) IsDBusStruct() bool { return false }

func (s *SelfMarshalerPtr) SignatureDBus() Signature {
	return mustSignatureFor[uint16]()
}

// VarDict is a struct that marshals to a DBus dict of string to
// variant.
type VarDict struct {
	A uint16 `dbus:"key=foo"`
	B uint32 `dbus:"key=bar,encodeZero"`
	C string `dbus:"key=@"`
	D uint8  `dbus:"key=@"`

	Other map[string]Variant `dbus:"vardict"`
}

// VarDictByte is a struct that marshals to a DBus dict of byte to
// variant.
type VarDictByte struct {
	A uint16 `dbus:"key=1"`
	B string `dbus:"key=2"`

	Other map[byte]Variant `dbus:"vardict"`
}

func ptr[T any](v T) *T {
	return &v
}

func mustSignatureFor[T any]() Signature {
	sig, err := SignatureFor[T]()
	if err != nil {
		panic(err)
	}
	return sig
}
