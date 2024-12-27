package fragments

import (
	"encoding/binary"
	"errors"
	"reflect"
)

type EncoderFunc func(enc *Encoder, val reflect.Value) error

type Encoder struct {
	Order  binary.AppendByteOrder
	Mapper func(reflect.Type) EncoderFunc
	Out    []byte
}

func (e *Encoder) Pad(align int) {
	extra := len(e.Out) % align
	if extra == 0 {
		return
	}
	var pad [8]byte
	e.Out = append(e.Out, pad[:align-extra]...)
	return
}

func (e *Encoder) Bytes(bs []byte) {
	e.Out = append(e.Out, bs...)
}

func (e *Encoder) String(s string) {
	e.Out = append(e.Out, s...)
}

func (e *Encoder) Uint8(u8 uint8) {
	e.Out = append(e.Out, u8)
}

func (e *Encoder) Uint16(u16 uint16) {
	e.Out = e.Order.AppendUint16(e.Out, u16)
}

func (e *Encoder) Uint32(u32 uint32) {
	e.Out = e.Order.AppendUint32(e.Out, u32)
}

func (e *Encoder) Uint64(u64 uint64) {
	e.Out = e.Order.AppendUint64(e.Out, u64)
}

func (e *Encoder) Value(v any) error {
	if e.Mapper == nil {
		return errors.New("Mapper not provided to Encoder")
	}
	fn := e.Mapper(reflect.TypeOf(v))
	return fn(e, reflect.ValueOf(v))
}
