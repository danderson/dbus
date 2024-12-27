package dbus

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"reflect"

	"github.com/danderson/dbus/fragments"
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
	dec := decoders.GetRecover(val.Elem().Type())
	st := fragments.Decoder{
		Order: ord,
		In:    bs,
	}
	return dec(&st, val.Elem())
}

const debugDecoders = false

func debugDecoder(msg string, args ...any) {
	if !debugDecoders {
		return
	}
	log.Printf(msg, args...)
}

var decoders cache[fragments.DecoderFunc]

func init() {
	// This needs to be an init func to break the initialization cycle
	// between the cache and the calls to the cache within
	// uncachedTypeEncoder.
	decoders.Init(uncachedTypeDecoder, func(t reflect.Type) fragments.DecoderFunc {
		return newErrDecoder(t, "recursive type")
	})
}

func uncachedTypeDecoder(t reflect.Type) fragments.DecoderFunc {
	debugDecoder("typeDecoder(%s)", t)
	defer debugDecoder("end typeDecoder(%s)", t)

	// TODO: pointer marshaler optimization thing?

	switch t.Kind() {
	case reflect.Pointer:
		return newPtrDecoder(t)
	case reflect.Bool:
		return newBoolDecoder()
	case reflect.Int, reflect.Uint:
		return newErrDecoder(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntDecoder(t)
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newUintDecoder(t)
	case reflect.Float32, reflect.Float64:
		return newFloatDecoder()
	case reflect.String:
		return newStringDecoder()
	case reflect.Slice, reflect.Array:
		return newSliceDecoder(t)
	case reflect.Struct:
		return newStructDecoder(t)
	case reflect.Map:
		return newMapDecoder(t)
	}

	return newErrDecoder(t, "no known mapping")
}

func newErrDecoder(t reflect.Type, reason string) fragments.DecoderFunc {
	err := unrepresentable(t, reason)
	decoders.Unwind(func(*fragments.Decoder, reflect.Value) error {
		return err
	})
	// So that callers can return the result of this constructor and
	// pretend that it's not doing any non-local return. The non-local
	// return is just an optimization so that decoders don't waste
	// time partially decoding types that will never fully succeed.
	return nil
}

func newPtrDecoder(t reflect.Type) fragments.DecoderFunc {
	debugDecoder("ptr{%s}", t.Elem())
	elem := t.Elem()
	elemDec := decoders.Get(elem)
	return func(st *fragments.Decoder, v reflect.Value) error {
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
	}
}

func newBoolDecoder() fragments.DecoderFunc {
	debugDecoder("bool{}")
	return func(st *fragments.Decoder, v reflect.Value) error {
		st.Pad(4)
		u, err := st.Uint32()
		if err != nil {
			return err
		}
		v.SetBool(u != 0)
		return nil
	}
}

func newIntDecoder(t reflect.Type) fragments.DecoderFunc {
	switch t.Size() {
	case 1:
		debugDecoder("int8{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			u8, err := st.Uint8()
			if err != nil {
				return err
			}
			v.SetInt(int64(int8(u8)))
			return nil
		}
	case 2:
		debugDecoder("int16{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			st.Pad(2)
			u16, err := st.Uint16()
			if err != nil {
				return err
			}
			v.SetInt(int64(int16(u16)))
			return nil
		}
	case 4:
		debugDecoder("int32{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			st.Pad(4)
			u32, err := st.Uint32()
			if err != nil {
				return err
			}
			v.SetInt(int64(int32(u32)))
			return nil
		}
	case 8:
		debugDecoder("int64{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			st.Pad(8)
			u64, err := st.Uint64()
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

func newUintDecoder(t reflect.Type) fragments.DecoderFunc {
	switch t.Size() {
	case 1:
		debugDecoder("uint8{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			u8, err := st.Uint8()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u8))
			return nil
		}
	case 2:
		debugDecoder("uint16{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			st.Pad(2)
			u16, err := st.Uint16()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u16))
			return nil
		}
	case 4:
		debugDecoder("uint32{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			st.Pad(4)
			u32, err := st.Uint32()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u32))
			return nil
		}
	case 8:
		debugDecoder("uint64{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			st.Pad(8)
			u64, err := st.Uint64()
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

func newFloatDecoder() fragments.DecoderFunc {
	debugDecoder("float64{}")
	return func(st *fragments.Decoder, v reflect.Value) error {
		st.Pad(8)
		u64, err := st.Uint64()
		if err != nil {
			return err
		}
		v.SetFloat(math.Float64frombits(u64))
		return nil
	}
}

func newStringDecoder() fragments.DecoderFunc {
	debugDecoder("string{}")
	return func(st *fragments.Decoder, v reflect.Value) error {
		st.Pad(4)
		u32, err := st.Uint32()
		if err != nil {
			return err
		}
		ret, err := st.String(int(u32))
		if err != nil {
			return err
		}
		if _, err := st.Uint8(); err != nil {
			return err
		}
		v.SetString(ret)
		return nil
	}
}

func newSliceDecoder(t reflect.Type) fragments.DecoderFunc {
	if t.Elem().Kind() == reflect.Uint8 {
		debugDecoder("[]byte{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			st.Pad(4)
			u32, err := st.Uint32()
			if err != nil {
				return err
			}
			ret, err := st.Bytes(int(u32))
			if err != nil {
				return err
			}
			v.SetBytes(ret)
			return nil
		}
	}

	debugDecoder("[]%s{}", t.Elem())
	elemDec := decoders.Get(t.Elem())

	return func(st *fragments.Decoder, v reflect.Value) error {
		st.Pad(4)
		u32, err := st.Uint32()
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
	}
}

type structFieldDecoder struct {
	idx [][]int
	dec fragments.DecoderFunc
}

type structDecoder []structFieldDecoder

func (fs structDecoder) decode(st *fragments.Decoder, v reflect.Value) error {
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

func newStructDecoder(t reflect.Type) fragments.DecoderFunc {
	debugDecoder("%s{}", t)
	ret := structDecoder{}
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous || !f.IsExported() {
			continue
		}
		debugDecoder("%s.%s{%s}", t, f.Name, f.Type)
		fDec := decoders.Get(f.Type)
		fd := structFieldDecoder{
			idx: allocSteps(t, f.Index),
			dec: fDec,
		}

		ret = append(ret, fd)
	}
	if len(ret) == 0 {
		return newErrDecoder(t, "no exported struct fields")
	}
	return ret.decode
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

func newMapDecoder(t reflect.Type) fragments.DecoderFunc {
	debugDecoder("map[%s]%s{}", t.Key(), t.Elem())
	kt := t.Key()
	switch kt.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16, reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
	default:
		return newErrDecoder(t, fmt.Sprintf("invalid map key type %s", kt))
	}
	kDec := decoders.Get(kt)
	vt := t.Elem()
	vDec := decoders.Get(vt)

	return func(st *fragments.Decoder, v reflect.Value) error {
		st.Pad(4)
		u32, err := st.Uint32()
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
	}
}
