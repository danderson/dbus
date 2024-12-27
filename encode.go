package dbus

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

func Marshal(v any, ord binary.AppendByteOrder) ([]byte, error) {
	return MarshalAppend(nil, v, ord)
}

func MarshalAppend(bs []byte, v any, ord binary.AppendByteOrder) ([]byte, error) {
	val := reflect.ValueOf(v)
	enc := encoders.GetRecover(val.Type())
	st := fragments.Encoder{
		Order:  ord,
		Mapper: encoders.Get,
		Out:    bs,
	}
	if err := enc(&st, val); err != nil {
		return nil, err
	}
	return st.Out, nil
}

const debugEncoders = false

func debugEncoder(msg string, args ...any) {
	if !debugEncoders {
		return
	}
	log.Printf(msg, args...)
}

type Marshaler interface {
	SignatureDBus() Signature
	AlignDBus() int
	MarshalDBus(st *fragments.Encoder) error
}

var marshalerType = reflect.TypeFor[Marshaler]()

var encoders cache[fragments.EncoderFunc]

func init() {
	// This needs to be an init func to break the initialization cycle
	// between the cache and the calls to the cache within
	// uncachedTypeEncoder.
	encoders.Init(uncachedTypeEncoder, func(t reflect.Type) fragments.EncoderFunc {
		return newErrEncoder(t, "recursive type")
	})
}

func uncachedTypeEncoder(t reflect.Type) (ret fragments.EncoderFunc) {
	debugEncoder("typeEncoder(%s)", t)
	defer debugEncoder("end typeEncoder(%s)", t)

	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(marshalerType) {
		return newCondAddrMarshalEncoder(t)
	} else if t.Implements(marshalerType) {
		return newMarshalEncoder(t)
	}

	switch t.Kind() {
	case reflect.Pointer:
		return newPtrEncoder(t)
	case reflect.Bool:
		return newBoolEncoder()
	case reflect.Int, reflect.Uint:
		return newErrEncoder(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntEncoder(t)
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newUintEncoder(t)
	case reflect.Float32, reflect.Float64:
		return newFloatEncoder()
	case reflect.String:
		return newStringEncoder()
	case reflect.Slice, reflect.Array:
		return newSliceEncoder(t)
	case reflect.Struct:
		return newStructEncoder(t)
	case reflect.Map:
		return newMapEncoder(t)
	}
	return newErrEncoder(t, "no known mapping")
}

func newCondAddrMarshalEncoder(t reflect.Type) fragments.EncoderFunc {
	ptr := newMarshalEncoder(reflect.PointerTo(t))
	if t.Implements(marshalerType) {
		debugEncoder("%s{} (external marshaler, w/ addressable optimization)", t)
		val := newMarshalEncoder(t)
		return func(st *fragments.Encoder, v reflect.Value) error {
			if v.CanAddr() {
				return ptr(st, v.Addr())
			} else {
				return val(st, v)
			}
		}
	} else {
		debugEncoder("%s{} (external marshaler, addressable only)", t)
		return func(st *fragments.Encoder, v reflect.Value) error {
			if !v.CanAddr() {
				return unrepresentable(t, "Marshaler only implemented on pointer receiver, and cannot take address of value")
			}
			return ptr(st, v.Addr())
		}
	}
}

func newErrEncoder(t reflect.Type, reason string) fragments.EncoderFunc {
	err := unrepresentable(t, reason)
	encoders.Unwind(func(*fragments.Encoder, reflect.Value) error {
		return err
	})
	// So that callers can return the result of this constructor and
	// pretend that it's not doing any non-local return. The non-local
	// return is just an optimization so that encoders don't waste
	// time partially encoding types that will never fully succeed.
	return nil
}

func newMarshalEncoder(t reflect.Type) fragments.EncoderFunc {
	debugEncoder("%s{} (external Marshaler)", t)
	return func(st *fragments.Encoder, v reflect.Value) error {
		m := v.Interface().(Marshaler)
		st.Pad(m.AlignDBus())
		return m.MarshalDBus(st)
	}
}

func newPtrEncoder(t reflect.Type) fragments.EncoderFunc {
	debugEncoder("ptr{%s}", t.Elem())
	elemEnc := encoders.Get(t.Elem())
	return func(st *fragments.Encoder, v reflect.Value) error {
		if v.IsNil() {
			return elemEnc(st, reflect.Zero(t))
		}
		return elemEnc(st, v.Elem())
	}
}

func newBoolEncoder() fragments.EncoderFunc {
	debugEncoder("bool{}")
	return func(st *fragments.Encoder, v reflect.Value) error {
		st.Pad(4)
		val := uint32(0)
		if v.Bool() {
			val = 1
		}
		st.Uint32(val)
		return nil
	}
}

func newIntEncoder(t reflect.Type) fragments.EncoderFunc {
	switch t.Size() {
	case 1:
		debugEncoder("int8{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Uint8(byte(v.Int()))
			return nil
		}
	case 2:
		debugEncoder("int16{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Pad(2)
			st.Uint16(uint16(v.Int()))
			return nil
		}
	case 4:
		debugEncoder("int32{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Pad(4)
			st.Uint32(uint32(v.Int()))
			return nil
		}
	case 8:
		debugEncoder("int64{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Pad(8)
			st.Uint64(uint64(v.Int()))
			return nil
		}
	default:
		panic("invalid newIntEncoder type")
	}
}

func newUintEncoder(t reflect.Type) fragments.EncoderFunc {
	switch t.Size() {
	case 1:
		debugEncoder("uint8{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Uint8(uint8(v.Uint()))
			return nil
		}
	case 2:
		debugEncoder("uint16{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Pad(2)
			st.Uint16(uint16(v.Uint()))
			return nil
		}
	case 4:
		debugEncoder("uint32{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Pad(4)
			st.Uint32(uint32(v.Uint()))
			return nil
		}
	case 8:
		debugEncoder("uint64{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			st.Pad(8)
			st.Uint64(v.Uint())
			return nil
		}
	default:
		panic("invalid newIntEncoder type")
	}
}

func newFloatEncoder() fragments.EncoderFunc {
	debugEncoder("float64{}")
	return func(st *fragments.Encoder, v reflect.Value) error {
		st.Pad(8)
		st.Uint64(math.Float64bits(v.Float()))
		return nil
	}
}

func newStringEncoder() fragments.EncoderFunc {
	debugEncoder("string{}")
	return func(st *fragments.Encoder, v reflect.Value) error {
		s := v.String()
		st.Pad(4)
		st.Uint32(uint32(len(s)))
		st.String(s)
		st.Uint8(0)
		return nil
	}
}

func newSliceEncoder(t reflect.Type) fragments.EncoderFunc {
	if t.Elem().Kind() == reflect.Uint8 {
		debugEncoder("[]byte{}")
		return func(st *fragments.Encoder, v reflect.Value) error {
			bs := v.Bytes()
			st.Pad(4)
			st.Uint32(uint32(len(bs)))
			st.Bytes(bs)
			return nil
		}
	}

	debugEncoder("[]%s{}", t.Elem())
	elemEnc := encoders.Get(t.Elem())

	return func(st *fragments.Encoder, v reflect.Value) error {
		ln := v.Len()
		st.Pad(4)
		st.Uint32(uint32(ln))
		st.Pad(arrayPad(t.Elem()))
		for i := 0; i < ln; i++ {
			if err := elemEnc(st, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}
}

func arrayPad(elem reflect.Type) int {
	if elem.Implements(marshalerType) {
		return reflect.Zero(elem).Interface().(Marshaler).AlignDBus()
	} else if ptr := reflect.PointerTo(elem); ptr.Implements(marshalerType) {
		return reflect.Zero(ptr).Interface().(Marshaler).AlignDBus()
	} else {
		switch elem.Kind() {
		case reflect.Int8, reflect.Uint8:
			return 1
		case reflect.Int16, reflect.Uint16:
			return 2
		case reflect.Bool, reflect.Int32, reflect.Uint32, reflect.Slice, reflect.Array, reflect.String:
			return 4
		case reflect.Int64, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.Struct:
			return 8
		default:
			panic(fmt.Sprintf("missing array pad value for %s", elem))
		}
	}
}

type structFieldEncoder struct {
	idx []int
	enc fragments.EncoderFunc
}

type structEncoder []structFieldEncoder

func (fs structEncoder) encode(st *fragments.Encoder, v reflect.Value) error {
	st.Pad(8)

	for _, f := range fs {
		fv := v.FieldByIndex(f.idx)
		if err := f.enc(st, fv); err != nil {
			return err
		}
	}
	return nil
}

func newStructEncoder(t reflect.Type) fragments.EncoderFunc {
	debugEncoder("%s{}", t)
	ret := structEncoder{}
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous || !f.IsExported() {
			continue
		}
		debugEncoder("%s.%s{%s}", t, f.Name, f.Type)
		fEnc := encoders.Get(f.Type)
		ret = append(ret, structFieldEncoder{f.Index, fEnc})
	}
	if len(ret) == 0 {
		return newErrEncoder(t, "no exported struct fields")
	}
	return ret.encode
}

func newMapEncoder(t reflect.Type) fragments.EncoderFunc {
	debugEncoder("map[%s]%s{}", t.Key(), t.Elem())
	kt := t.Key()
	switch kt.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16, reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
	default:
		return newErrEncoder(t, fmt.Sprintf("unrepresentable map key type %s", kt))
	}
	kEnc := encoders.Get(kt)
	vt := t.Elem()
	vEnc := encoders.Get(vt)

	return func(st *fragments.Encoder, v reflect.Value) error {
		ln := v.Len()
		st.Pad(4)
		st.Uint32(uint32(ln))
		st.Pad(8)
		iter := v.MapRange()
		for iter.Next() {
			st.Pad(8)
			if err := kEnc(st, iter.Key()); err != nil {
				return err
			}
			if err := vEnc(st, iter.Value()); err != nil {
				return err
			}
		}
		return nil
	}
}
