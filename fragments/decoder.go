package fragments

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
)

type DecoderFunc func(dec *Decoder, val reflect.Value) error

type Decoder struct {
	Order  binary.ByteOrder
	Mapper func(reflect.Type) DecoderFunc
	In     []byte
	offset int
}

func (d *Decoder) advance(n int) {
	d.In = d.In[n:]
	d.offset += n
}

func (d *Decoder) Pad(align int) {
	extra := d.offset % align
	if extra == 0 {
		return
	}
	d.advance(align - extra)
}

func (d *Decoder) Bytes(n int) ([]byte, error) {
	if len(d.In) < n {
		return nil, io.ErrUnexpectedEOF
	}
	ret := d.In[:n]
	d.advance(n)
	return ret, nil
}

func (d *Decoder) String(n int) (string, error) {
	bs, err := d.Bytes(n)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (d *Decoder) UnmarshalUint8() (uint8, error) {
	if len(d.In) < 1 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.In[0]
	d.advance(1)
	return ret, nil
}

func (d *Decoder) Uint8() (uint8, error) {
	if len(d.In) < 1 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.In[0]
	d.advance(1)
	return ret, nil
}

func (d *Decoder) Uint16() (uint16, error) {
	if len(d.In) < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.Order.Uint16(d.In)
	d.advance(2)
	return ret, nil
}

func (d *Decoder) Uint32() (uint32, error) {
	if len(d.In) < 4 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.Order.Uint32(d.In)
	d.advance(4)
	return ret, nil
}

func (d *Decoder) Uint64() (uint64, error) {
	if len(d.In) < 8 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.Order.Uint64(d.In)
	d.advance(8)
	return ret, nil
}

func (d *Decoder) Value(v any) error {
	if d.Mapper == nil {
		return errors.New("Mapper not provided to Decoder")
	}
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Pointer {
		return fmt.Errorf("outval of Decoder.Value must be a pointer, got %s", t)
	}
	fn := d.Mapper(t.Elem())
	return fn(d, reflect.ValueOf(v).Elem())
}
