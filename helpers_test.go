package dbus_test

import (
	"github.com/danderson/dbus"
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

// NestedSelfMashalerVal is a struct with a struct field that
// implements dbus.Marshaler/dbus.Unmarshaler with value method
// receivers. NestedSelfMashalerVal cannot be unmarshaled, because
// UnmarshalDBus must be implemented on a pointer receiver.
type NestedSelfMashalerVal struct {
	A byte
	B SelfMarshalerVal
}

// NestedSelfMarshalerPtr is a struct with a struct field that
// implements dbus.Marshaler/dbus.Unmarshaler with pointer method
// receivers.
type NestedSelfMarshalerPtr struct {
	A byte
	B SelfMarshalerPtr
}

// NestedSelfMarshalerPtrPtr is a struct with a struct pointer field
// that implements dbus.Marshaler/dbus.Unmarshaler with pointer method
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

// SelfMarshalerVal is a struct that implements dbus.Marshaler and
// dbus.Unmarshaler, with value method receivers. Note the
// dbus.Unmarshaler implementation is deliberately unusable
// (UnmarshalDBus must have a pointer receiver).
type SelfMarshalerVal struct {
	B byte
}

func (s SelfMarshalerVal) MarshalDBus(st *fragments.Encoder) error {
	st.Pad(3)
	st.Uint16(uint16(s.B) + 1)
	return nil
}

func (s SelfMarshalerVal) UnmarshalDBus(st *fragments.Decoder) error {
	st.Pad(3)
	u16, err := st.Uint16()
	if err != nil {
		return err
	}
	s.B = byte(u16) - 1
	return nil
}

func (s SelfMarshalerVal) AlignDBus() int { return 3 }

func (s SelfMarshalerVal) SignatureDBus() dbus.Signature { return "q" }

// SelfMarshalerPtr is a struct that implements dbus.Marshaler and
// dbus.Unmarshaler with pointer method receivers.
type SelfMarshalerPtr struct {
	B byte
}

func (s *SelfMarshalerPtr) MarshalDBus(st *fragments.Encoder) error {
	st.Pad(3)
	st.Uint16(uint16(s.B) + 1)
	return nil
}

func (s *SelfMarshalerPtr) UnmarshalDBus(st *fragments.Decoder) error {
	st.Pad(3)
	u16, err := st.Uint16()
	if err != nil {
		return err
	}
	s.B = byte(u16) - 1
	return nil
}

func (s *SelfMarshalerPtr) AlignDBus() int { return 3 }

func (s *SelfMarshalerPtr) SignatureDBus() dbus.Signature { return "q" }
