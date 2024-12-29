package fragments

import (
	"errors"
	"reflect"
)

// An EncoderFunc writes a value to the given encoder.
type EncoderFunc func(enc *Encoder, val reflect.Value) error

// An Encoder provides utilities to write a DBus wire format message
// to a byte slice.
//
// Methods insert padding as needed to conform to DBus alignment
// rules, except for [Encoder.Write] which outputs bytes verbatim.
type Encoder struct {
	// Order is the byte order to use when encoding multi-byte values.
	Order ByteOrder
	// Mapper provides [EncoderFunc]s for types given to
	// [Encoder.Value]. If mapper is nil, the Encoder functions
	// normally except that [Encoder.Value] always returns an error.
	Mapper func(reflect.Type) EncoderFunc
	// Out is the encoded output.
	Out []byte
}

// Pad inserts padding bytes as needed to make the message a multiple
// of align bytes. If the message is already correctly aligned, no
// padding is inserted.
func (e *Encoder) Pad(align int) {
	extra := len(e.Out) % align
	if extra == 0 {
		return
	}
	var pad [8]byte
	e.Out = append(e.Out, pad[:align-extra]...)
	return
}

// Write writes bs as-is to the output. It is the caller's
// responsibility to ensure correct padding and encoding.
func (e *Encoder) Write(bs []byte) {
	e.Out = append(e.Out, bs...)
}

// Bytes writes bs to the output.
func (e *Encoder) Bytes(bs []byte) {
	e.Pad(4)
	e.Uint32(uint32(len(bs)))
	e.Out = append(e.Out, bs...)
}

// String writes s to the output.
func (e *Encoder) String(s string) {
	e.Pad(4)
	e.Uint32(uint32(len(s)))
	e.Out = append(e.Out, s...)
	e.Out = append(e.Out, 0)
}

// Uint8 writes a uint8.
func (e *Encoder) Uint8(u8 uint8) {
	e.Out = append(e.Out, u8)
}

// Uint16 writes uint16.
func (e *Encoder) Uint16(u16 uint16) {
	e.Pad(2)
	e.Out = e.Order.AppendUint16(e.Out, u16)
}

// Uint32 writes uint32.
func (e *Encoder) Uint32(u32 uint32) {
	e.Pad(4)
	e.Out = e.Order.AppendUint32(e.Out, u32)
}

// Uint64 writes uint64.
func (e *Encoder) Uint64(u64 uint64) {
	e.Pad(8)
	e.Out = e.Order.AppendUint64(e.Out, u64)
}

// Value writes v to the output, using the [EncoderFunc] provided by
// [Encoder.Mapper].
func (e *Encoder) Value(v any) error {
	if e.Mapper == nil {
		return errors.New("Mapper not provided to Encoder")
	}
	fn := e.Mapper(reflect.TypeOf(v))
	return fn(e, reflect.ValueOf(v))
}

// Array writes the header of an array of length elements.
//
// containsStructs indicates whether the array's elements are structs,
// so that the array header can be padded accordingly even if the
// array contains no elements.
//
// containsStructs only affects the size and alignment of the struct
// header. When writing an array of structs, the caller must also call
// [Encoder.Struct] to align each array element correctly.
func (e *Encoder) Array(length int, containsStructs bool) {
	e.Pad(4)
	e.Uint32(uint32(length))
	if containsStructs {
		e.Struct()
	}
}

// Struct aligns the output suitably for the start of a struct.
func (e *Encoder) Struct() {
	e.Pad(8)
}

// ByteOrderFlag writes the DBus byte order flag byte ('l' or 'B')
// that matches [Encoder.Order].
func (e *Encoder) ByteOrderFlag() {
	e.Write([]byte{e.Order.dbusFlag()})
}
