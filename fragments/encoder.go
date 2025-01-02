package fragments

import (
	"context"
	"errors"
	"reflect"
)

// An EncoderFunc writes a value to the given encoder.
type EncoderFunc func(ctx context.Context, enc *Encoder, val reflect.Value) error

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
func (e *Encoder) Value(ctx context.Context, v any) error {
	if e.Mapper == nil {
		return errors.New("Mapper not provided to Encoder")
	}
	fn := e.Mapper(reflect.TypeOf(v))
	return fn(ctx, e, reflect.ValueOf(v))
}

// Array writes an array to the output.
//
// Array elements must be added within the provided elements
// function. The elements function is responsible for padding each
// array element to the correct alignment for the element type.
//
// containsStructs indicates whether the array's elements are structs,
// so that the array header can be padded accordingly.
func (e *Encoder) Array(containsStructs bool, elements func() error) error {
	e.Pad(4)
	offset := len(e.Out)
	e.Uint32(0)
	if containsStructs {
		e.Pad(8)
	}

	start := len(e.Out)
	err := elements()
	end := len(e.Out)
	e.Order.PutUint32(e.Out[offset:], uint32(end-start))

	return err
}

// Struct writes a struct to the output.
//
// Struct fields must be added within the provided elements function.
func (e *Encoder) Struct(elements func() error) error {
	e.Pad(8)
	return elements()
}

// ByteOrderFlag writes the DBus byte order flag byte ('l' or 'B')
// that matches [Encoder.Order].
func (e *Encoder) ByteOrderFlag() {
	e.Write([]byte{e.Order.dbusFlag()})
}
