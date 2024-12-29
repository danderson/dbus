package fragments

import (
	"errors"
	"fmt"
	"io"
	"reflect"
)

// A DecoderFunc reads a value into val.
type DecoderFunc func(dec *Decoder, val reflect.Value) error

// A Decoder provides utilities to read a DBus wire format message to
// a byte slice.
//
// Methods advance the read cursor as needed to account for the
// padding required by DBus alignment rules, except for [Decoder.Read]
// which reads bytes verbatim.
type Decoder struct {
	// Order is the byte order to use when reading multi-byte values.
	Order ByteOrder
	// Mapper provides [DecoderFunc]s for types given to
	// [Decoder.Value]. If mapper is nil, the Decoder functions
	// normally except that [Decoder.Value] always returns an error.
	Mapper func(reflect.Type) DecoderFunc
	// In is the input stream to read.
	In io.Reader

	// offset is the number of bytes consumed off the front of In so
	// far. We have to keep track of this because alignment depends on
	// the global offset within the message, and cannot be derived
	// from local context partway through decoding.
	offset int
}

func (d *Decoder) Discard(n int) error {
	return nil
}

// Pad consumes padding bytes as needed to make the next read happen
// at a multiple of align bytes. If the decoder is already correctly
// aligned, no bytes are consumed.
func (d *Decoder) Pad(align int) error {
	extra := d.offset % align
	if extra == 0 {
		return nil
	}
	skip := align - extra
	if _, err := io.CopyN(io.Discard, d.In, int64(skip)); err != nil {
		return err
	}
	d.offset = (d.offset + skip) % 8
	return nil
}

// Read reads n bytes, with no framing or padding.
func (d *Decoder) Read(n int) ([]byte, error) {
	bs := make([]byte, n)
	if _, err := io.ReadFull(d.In, bs); err != nil {
		return nil, err
	}
	d.offset = (d.offset + n) % 8
	return bs, nil
}

// Bytes reads a DBus byte array.
func (d *Decoder) Bytes() ([]byte, error) {
	ln, err := d.Uint32()
	if err != nil {
		return nil, err
	}
	return d.Read(int(ln))
}

// Bytes reads a DBus string.
func (d *Decoder) String() (string, error) {
	ln, err := d.Uint32()
	if err != nil {
		return "", err
	}
	ret, err := d.Read(int(ln) + 1)
	if err != nil {
		return "", err
	}
	return string(ret[:len(ret)-1]), nil
}

// Uint8 reads a uint8.
func (d *Decoder) Uint8() (uint8, error) {
	bs, err := d.Read(1)
	if err != nil {
		return 0, err
	}
	return bs[0], nil
}

// Uint16 reads a uint16.
func (d *Decoder) Uint16() (uint16, error) {
	d.Pad(2)
	bs, err := d.Read(2)
	if err != nil {
		return 0, err
	}
	return d.Order.Uint16(bs), nil
}

// Uint32 reads a uint32.
func (d *Decoder) Uint32() (uint32, error) {
	d.Pad(4)
	bs, err := d.Read(4)
	if err != nil {
		return 0, err
	}
	return d.Order.Uint32(bs), nil
}

// Uint64 reads a uint64.
func (d *Decoder) Uint64() (uint64, error) {
	d.Pad(8)
	bs, err := d.Read(8)
	if err != nil {
		return 0, err
	}
	return d.Order.Uint64(bs), nil
}

// Value reads a value into v, using the [DecoderFunc] provided by
// [Decoder.Mapper]. v must be a non-nil pointer.
func (d *Decoder) Value(v any) error {
	if d.Mapper == nil {
		return errors.New("Mapper not provided to Decoder")
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer {
		return fmt.Errorf("outval of Decoder.Value must be a pointer, got %s", rv.Type())
	}
	if rv.IsNil() {
		return fmt.Errorf("outval of Decoder.Value must not be a nil pointer")
	}
	fn := d.Mapper(rv.Type().Elem())
	return fn(d, rv.Elem())
}

// Array reads the header of an array and returns the number of
// elements in the array.
//
// containsStructs indicates whether the array's elements are structs,
// so that the decoder consumes padding appropriately even if the
// array contains no elements.
//
// containsStructs only affects the size and alignment of the struct
// header. When reading an array of structs, the caller must also call
// [Decoder.Struct] to align with each array element correctly.
func (d *Decoder) Array(containsStructs bool) (int, error) {
	ln, err := d.Uint32()
	if err != nil {
		return 0, err
	}
	if containsStructs {
		if err := d.Struct(); err != nil {
			return 0, err
		}
	}
	return int(ln), nil
}

// Struct aligns the input suitably for the start of a struct.
func (d *Decoder) Struct() error {
	return d.Pad(8)
}

// ByteOrderFlag reads a DBus byte order flag byte, and sets
// [Decoder.Order] to match it.
func (d *Decoder) ByteOrderFlag() error {
	v, err := d.Uint8()
	if err != nil {
		return err
	}
	switch v {
	case 'B':
		d.Order = BigEndian
	case 'l':
		d.Order = LittleEndian
	default:
		return fmt.Errorf("unknown byte order flag %q", v)
	}
	return nil
}
