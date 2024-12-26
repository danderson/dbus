package dbus

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"reflect"
	"sync"
)

func Unmarshal(bs []byte, ord binary.ByteOrder, v any) error {
	if v == nil {
		return fmt.Errorf("can't unmarshal into nil interface")
	}
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer {
		return fmt.Errorf("can't unmarshal into a non-pointer")
	}
	if val.IsNil() {
		return fmt.Errorf("can't unmarshal into nil pointer")
	}
	dec, err := typeDecoder(val.Elem().Type())
	if err != nil {
		return err
	}
	st := decodeState{ord, 0, bs}
	return dec(&st, val.Elem())
}

type decodeState struct {
	ord    binary.ByteOrder
	offset int
	raw    []byte
}

func (d *decodeState) advance(n int) {
	d.offset += n
	d.raw = d.raw[n:]
}

func (d *decodeState) Pad(align int) {
	extra := d.offset % align
	if extra == 0 {
		return
	}
	d.advance(align - extra)
}

func (d *decodeState) Bytes(n int) ([]byte, error) {
	if len(d.raw) < n {
		return nil, io.ErrUnexpectedEOF
	}
	ret := d.raw[:n]
	d.advance(n)
	return ret, nil
}

func (d *decodeState) String(n int) (string, error) {
	bs, err := d.Bytes(n)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (d *decodeState) UnmarshalUint8() (uint8, error) {
	if len(d.raw) < 1 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.raw[0]
	d.advance(1)
	return ret, nil
}

func (d *decodeState) UnmarshalUint16() (uint16, error) {
	if len(d.raw) < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.ord.Uint16(d.raw)
	d.advance(2)
	return ret, nil
}

func (d *decodeState) UnmarshalUint32() (uint32, error) {
	if len(d.raw) < 4 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.ord.Uint32(d.raw)
	d.advance(4)
	return ret, nil
}

func (d *decodeState) UnmarshalUint64() (uint64, error) {
	if len(d.raw) < 8 {
		return 0, io.ErrUnexpectedEOF
	}
	ret := d.ord.Uint64(d.raw)
	d.advance(8)
	return ret, nil
}

type decoderFunc func(*decodeState, reflect.Value) error

var decoderCache sync.Map

const debugDecoders = false

func debugDecoder(msg string, args ...any) {
	if !debugDecoders {
		return
	}
	log.Printf(msg, args...)
}

func typeDecoder(t reflect.Type) (ret decoderFunc, err error) {
	debugDecoder("typeDecoder(%s)", t)
	defer debugDecoder("end typeDecoder(%s)", t)
	if cached, loaded := decoderCache.LoadOrStore(t, nil); loaded {
		if cached == nil {
			err := unrepresentable(t, "recursive type")
			decoderCache.CompareAndSwap(t, nil, err)
			return nil, err
		}
		if err, ok := cached.(error); ok {
			return nil, err
		}
		debugDecoder("%s{} (cached)", t)
		return cached.(decoderFunc), nil
	}

	defer func() {
		if err != nil {
			decoderCache.CompareAndSwap(t, nil, err)
		} else {
			decoderCache.CompareAndSwap(t, nil, ret)
		}
	}()

	// TODO: pointer marshaler optimization thing?

	return deriveTypeDecoder(t)
}

func deriveTypeDecoder(t reflect.Type) (decoderFunc, error) {
	switch t.Kind() {
	case reflect.Pointer:
		return newPtrDecoder(t)
	case reflect.Bool:
		return newBoolDecoder(), nil
	case reflect.Int, reflect.Uint:
		return nil, unrepresentable(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntDecoder(t), nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newUintDecoder(t), nil
	case reflect.Float32, reflect.Float64:
		return newFloatDecoder(), nil
	case reflect.String:
		return newStringDecoder(), nil
	case reflect.Slice, reflect.Array:
		return newSliceDecoder(t)
	case reflect.Struct:
		return newStructDecoder(t)
	case reflect.Map:
		return newMapDecoder(t)
	}

	return nil, unrepresentable(t, "no known mapping")
}

func newPtrDecoder(t reflect.Type) (decoderFunc, error) {
	debugDecoder("ptr{%s}", t.Elem())
	elem := t.Elem()
	elemDec, err := typeDecoder(elem)
	if err != nil {
		return nil, err
	}
	return func(st *decodeState, v reflect.Value) error {
		if v.IsNil() {
			elem := reflect.New(elem)
			if err := elemDec(st, elem.Elem()); err != nil {
				return err
			}
			v.Set(elem)
		} else if err := elemDec(st, v.Elem()); err != nil {
			return err
		}
		return nil
	}, nil
}

func newBoolDecoder() decoderFunc {
	debugDecoder("bool{}")
	return func(st *decodeState, v reflect.Value) error {
		st.Pad(4)
		u, err := st.UnmarshalUint32()
		if err != nil {
			return err
		}
		v.SetBool(u != 0)
		return nil
	}
}

func newIntDecoder(t reflect.Type) decoderFunc {
	switch t.Size() {
	case 1:
		debugDecoder("int8{}")
		return func(st *decodeState, v reflect.Value) error {
			u8, err := st.UnmarshalUint8()
			if err != nil {
				return err
			}
			v.SetInt(int64(int8(u8)))
			return nil
		}
	case 2:
		debugDecoder("int16{}")
		return func(st *decodeState, v reflect.Value) error {
			st.Pad(2)
			u16, err := st.UnmarshalUint16()
			if err != nil {
				return err
			}
			v.SetInt(int64(int16(u16)))
			return nil
		}
	case 4:
		debugDecoder("int32{}")
		return func(st *decodeState, v reflect.Value) error {
			st.Pad(4)
			u32, err := st.UnmarshalUint32()
			if err != nil {
				return err
			}
			v.SetInt(int64(int32(u32)))
			return nil
		}
	case 8:
		debugDecoder("int64{}")
		return func(st *decodeState, v reflect.Value) error {
			st.Pad(8)
			u64, err := st.UnmarshalUint64()
			if err != nil {
				return err
			}
			v.SetInt(int64(int64(u64)))
			return nil
		}
	default:
		panic("invalid newIntDecoder type")
	}
}

func newUintDecoder(t reflect.Type) decoderFunc {
	switch t.Size() {
	case 1:
		debugDecoder("uint8{}")
		return func(st *decodeState, v reflect.Value) error {
			u8, err := st.UnmarshalUint8()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u8))
			return nil
		}
	case 2:
		debugDecoder("uint16{}")
		return func(st *decodeState, v reflect.Value) error {
			st.Pad(2)
			u16, err := st.UnmarshalUint16()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u16))
			return nil
		}
	case 4:
		debugDecoder("uint32{}")
		return func(st *decodeState, v reflect.Value) error {
			st.Pad(4)
			u32, err := st.UnmarshalUint32()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u32))
			return nil
		}
	case 8:
		debugDecoder("uint64{}")
		return func(st *decodeState, v reflect.Value) error {
			st.Pad(8)
			u64, err := st.UnmarshalUint64()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u64))
			return nil
		}
	default:
		panic("invalid newUintDecoder type")
	}
}

func newFloatDecoder() decoderFunc {
	debugDecoder("float64{}")
	return func(st *decodeState, v reflect.Value) error {
		st.Pad(8)
		u64, err := st.UnmarshalUint64()
		if err != nil {
			return err
		}
		v.SetFloat(math.Float64frombits(u64))
		return nil
	}
}

func newStringDecoder() decoderFunc {
	debugDecoder("string{}")
	return func(st *decodeState, v reflect.Value) error {
		st.Pad(4)
		u32, err := st.UnmarshalUint32()
		if err != nil {
			return err
		}
		ret, err := st.String(int(u32))
		if err != nil {
			return err
		}
		if _, err := st.UnmarshalUint8(); err != nil {
			return err
		}
		v.SetString(ret)
		return nil
	}
}

func newSliceDecoder(t reflect.Type) (decoderFunc, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		debugDecoder("[]byte{}")
		return func(st *decodeState, v reflect.Value) error {
			st.Pad(4)
			u32, err := st.UnmarshalUint32()
			if err != nil {
				return err
			}
			ret, err := st.Bytes(int(u32))
			if err != nil {
				return err
			}
			v.SetBytes(ret)
			return nil
		}, nil
	}

	debugDecoder("[]%s{}", t.Elem())
	elemDec, err := typeDecoder(t.Elem())
	if err != nil {
		return nil, err
	}

	return func(st *decodeState, v reflect.Value) error {
		st.Pad(4)
		u32, err := st.UnmarshalUint32()
		if err != nil {
			return err
		}
		st.Pad(arrayPad(t.Elem()))

		v.Grow(int(u32))
		v.Set(v.Slice3(0, int(u32), int(u32)))
		for i := 0; i < int(u32); i++ {
			if err := elemDec(st, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}, nil
}

type structFieldDecoder struct {
	idx [][]int
	dec decoderFunc
}

type structDecoder []structFieldDecoder

func (fs structDecoder) decode(st *decodeState, v reflect.Value) error {
	st.Pad(8)
	for _, f := range fs {
		fv := v
		for i, hop := range f.idx {
			if i > 0 {
				if fv.IsNil() {
					fv.Set(reflect.New(fv.Type().Elem()))
				}
				fv = fv.Elem()
			}
			fv = fv.FieldByIndex(hop)
		}
		if err := f.dec(st, fv); err != nil {
			return err
		}
	}
	return nil
}

func newStructDecoder(t reflect.Type) (decoderFunc, error) {
	debugDecoder("%s{}", t)
	ret := structDecoder{}
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous || !f.IsExported() {
			continue
		}
		debugDecoder("%s.%s{%s}", t, f.Name, f.Type)
		fDec, err := typeDecoder(f.Type)
		if err != nil {
			return nil, err
		}
		fd := structFieldDecoder{
			idx: allocSteps(t, f.Index),
			dec: fDec,
		}

		ret = append(ret, fd)
	}
	if len(ret) == 0 {
		return nil, unrepresentable(t, "no exported struct fields")
	}
	return ret.decode, nil
}

// allocSteps partitions a multi-hop traversal of struct fields into
// segments that end at either the final value, or a struct pointer
// that might be nil.
//
// This can be used to traverse to idx while allocating missing
// structs, by using FieldByIndex repeatedly to traverse to each
// pointer and check for nil-ness.
func allocSteps(t reflect.Type, idx []int) [][]int {
	var ret [][]int
	prev := 0
	t = t.Field(idx[0]).Type
	for i := 1; i < len(idx); i++ {
		if t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct {
			// Hop through a struct pointer that might be nil, cut.
			ret = append(ret, idx[prev:i])
			prev = i
			t = t.Elem()
		}
		t = t.Field(idx[i]).Type
	}
	ret = append(ret, idx[prev:])
	return ret
}

func newMapDecoder(t reflect.Type) (decoderFunc, error) {
	debugDecoder("map[%s]%s{}", t.Key(), t.Elem())
	kt := t.Key()
	switch kt.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16, reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
	default:
		return nil, unrepresentable(t, fmt.Sprintf("unrepresentable map key type %s", kt))
	}
	kDec, err := typeDecoder(kt)
	if err != nil {
		return nil, err
	}
	vt := t.Elem()
	vDec, err := typeDecoder(vt)
	if err != nil {
		return nil, err
	}

	return func(st *decodeState, v reflect.Value) error {
		st.Pad(4)
		u32, err := st.UnmarshalUint32()
		if err != nil {
			return err
		}
		st.Pad(8)

		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		} else {
			v.Clear()
		}
		key := reflect.New(kt)
		val := reflect.New(vt)
		for i := 0; i < int(u32); i++ {
			key.Elem().SetZero()
			val.Elem().SetZero()
			st.Pad(8)
			if err := kDec(st, key.Elem()); err != nil {
				return err
			}
			if err := vDec(st, val.Elem()); err != nil {
				return err
			}
			v.SetMapIndex(key.Elem(), val.Elem())
		}
		return nil
	}, nil
}
