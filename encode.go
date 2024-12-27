package dbus

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"reflect"
	"sync"
)

func Marshal(v any, ord binary.AppendByteOrder) ([]byte, error) {
	return MarshalAppend(nil, v, ord)
}

func MarshalAppend(bs []byte, v any, ord binary.AppendByteOrder) ([]byte, error) {
	val := reflect.ValueOf(v)
	enc, err := typeEncoder(val.Type(), ord)
	if err != nil {
		return nil, err
	}
	return enc(bs, val)
}

type encoderFunc func(bs []byte, v reflect.Value) ([]byte, error)

var encoderCache sync.Map

type encoderCacheKey struct {
	t   reflect.Type
	ord binary.AppendByteOrder
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
	MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error)
}

var marshalerType = reflect.TypeFor[Marshaler]()

func typeEncoder(t reflect.Type, ord binary.AppendByteOrder) (ret encoderFunc, err error) {
	debugEncoder("typeEncoder(%s)", t)
	defer debugEncoder("end typeEncoder(%s)", t)
	k := encoderCacheKey{t, ord}
	if cached, loaded := encoderCache.LoadOrStore(k, nil); loaded {
		if cached == nil {
			err := unrepresentable(t, "recursive type")
			encoderCache.CompareAndSwap(k, nil, err)
			return nil, err
		}
		if err, ok := cached.(error); ok {
			return nil, err
		}
		debugEncoder("%s{} (cached)", t)
		return cached.(encoderFunc), nil
	}

	defer func() {
		if err != nil {
			encoderCache.CompareAndSwap(k, nil, err)
		} else {
			encoderCache.CompareAndSwap(k, nil, ret)
		}
	}()

	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(marshalerType) {
		return newCondAddrMarshalEncoder(t, ord)
	}
	return deriveTypeEncoder(t, ord)
}

func deriveTypeEncoder(t reflect.Type, ord binary.AppendByteOrder) (encoderFunc, error) {
	if t.Implements(marshalerType) {
		return newMarshalEncoder(t, ord), nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return newPtrEncoder(t, ord)
	case reflect.Bool:
		return newBoolEncoder(ord)
	case reflect.Int, reflect.Uint:
		return nil, unrepresentable(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntEncoder(t, ord), nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newUintEncoder(t, ord), nil
	case reflect.Float32, reflect.Float64:
		return newFloatEncoder(ord), nil
	case reflect.String:
		return newStringEncoder(ord), nil
	case reflect.Slice, reflect.Array:
		return newSliceEncoder(t, ord)
	case reflect.Struct:
		return newStructEncoder(t, ord)
	case reflect.Map:
		return newMapEncoder(t, ord)
	}
	return nil, unrepresentable(t, "no known mapping")
}

func newCondAddrMarshalEncoder(t reflect.Type, ord binary.AppendByteOrder) (encoderFunc, error) {
	var val encoderFunc
	if t.Implements(marshalerType) {
		debugEncoder("%s{} (external marshaler, w/ addressable optimization)", t)
		val = newMarshalEncoder(t, ord)
	} else {
		debugEncoder("%s{} (external marshaler, addressable only)", t)
		unrep := unrepresentable(t, "Marshaler only implemented on pointer receiver, and cannot take address of value")
		val = func(_ []byte, v reflect.Value) ([]byte, error) {
			return nil, unrep
		}
	}
	ptr := newMarshalEncoder(reflect.PointerTo(t), ord)

	return func(bs []byte, v reflect.Value) ([]byte, error) {
		if v.CanAddr() {
			return ptr(bs, v.Addr())
		} else {
			return val(bs, v)
		}
	}, nil
}

func newMarshalEncoder(t reflect.Type, ord binary.AppendByteOrder) encoderFunc {
	debugEncoder("%s{} (external Marshaler)", t)
	return func(bs []byte, v reflect.Value) ([]byte, error) {
		m := v.Interface().(Marshaler)
		bs = pad(bs, m.AlignDBus())
		return m.MarshalDBus(bs, ord)
	}
}

func newPtrEncoder(t reflect.Type, ord binary.AppendByteOrder) (encoderFunc, error) {
	debugEncoder("ptr{%s}", t.Elem())
	elemEnc, err := typeEncoder(t.Elem(), ord)
	if err != nil {
		return nil, err
	}
	return func(bs []byte, v reflect.Value) ([]byte, error) {
		if v.IsNil() {
			return elemEnc(bs, reflect.Zero(t))
		}
		return elemEnc(bs, v.Elem())
	}, nil
}

func newBoolEncoder(ord binary.AppendByteOrder) (encoderFunc, error) {
	debugEncoder("bool{}")
	return func(bs []byte, v reflect.Value) ([]byte, error) {
		bs = pad(bs, 4)
		val := uint32(0)
		if v.Bool() {
			val = 1
		}
		return ord.AppendUint32(bs, val), nil
	}, nil
}

func newIntEncoder(t reflect.Type, ord binary.AppendByteOrder) encoderFunc {
	switch t.Size() {
	case 1:
		debugEncoder("int8{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return append(bs, byte(v.Int())), nil
		}
	case 2:
		debugEncoder("int16{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return ord.AppendUint16(pad(bs, 2), uint16(v.Int())), nil
		}
	case 4:
		debugEncoder("int32{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return ord.AppendUint32(pad(bs, 4), uint32(v.Int())), nil
		}
	case 8:
		debugEncoder("int64{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return ord.AppendUint64(pad(bs, 8), uint64(v.Int())), nil
		}
	default:
		panic("invalid newIntEncoder type")
	}
}

func newUintEncoder(t reflect.Type, ord binary.AppendByteOrder) encoderFunc {
	switch t.Size() {
	case 1:
		debugEncoder("uint8{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return append(bs, byte(v.Uint())), nil
		}
	case 2:
		debugEncoder("uint16{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return ord.AppendUint16(pad(bs, 2), uint16(v.Uint())), nil
		}
	case 4:
		debugEncoder("uint32{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return ord.AppendUint32(pad(bs, 4), uint32(v.Uint())), nil
		}
	case 8:
		debugEncoder("uint64{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			return ord.AppendUint64(pad(bs, 8), uint64(v.Uint())), nil
		}
	default:
		panic("invalid newIntEncoder type")
	}
}

func newFloatEncoder(ord binary.AppendByteOrder) encoderFunc {
	debugEncoder("float64{}")
	return func(bs []byte, v reflect.Value) ([]byte, error) {
		return ord.AppendUint64(pad(bs, 8), math.Float64bits(v.Float())), nil
	}
}

func newStringEncoder(ord binary.AppendByteOrder) encoderFunc {
	debugEncoder("string{}")
	return func(bs []byte, v reflect.Value) ([]byte, error) {
		s := v.String()
		bs = ord.AppendUint32(pad(bs, 4), uint32(len(s)))
		bs = append(bs, s...)
		bs = append(bs, 0)
		return bs, nil
	}
}

func newSliceEncoder(t reflect.Type, ord binary.AppendByteOrder) (encoderFunc, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		debugEncoder("[]byte{}")
		return func(bs []byte, v reflect.Value) ([]byte, error) {
			val := v.Bytes()
			bs = ord.AppendUint32(pad(bs, 4), uint32(len(val)))
			bs = append(bs, val...)
			return bs, nil
		}, nil
	}

	debugEncoder("[]%s{}", t.Elem())
	elemEnc, err := typeEncoder(t.Elem(), ord)
	if err != nil {
		return nil, err
	}

	return func(bs []byte, v reflect.Value) ([]byte, error) {
		ln := v.Len()
		bs = ord.AppendUint32(pad(bs, 4), uint32(ln))
		bs = pad(bs, arrayPad(t.Elem()))
		for i := 0; i < ln; i++ {
			var err error
			bs, err = elemEnc(bs, v.Index(i))
			if err != nil {
				return nil, err
			}
		}
		return bs, nil
	}, nil
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
	enc encoderFunc
}

type structEncoder []structFieldEncoder

func (fs structEncoder) encode(bs []byte, v reflect.Value) ([]byte, error) {
	bs = pad(bs, 8)

	var err error
	for _, f := range fs {
		fv := v.FieldByIndex(f.idx)
		bs, err = f.enc(bs, fv)
		if err != nil {
			return nil, err
		}
	}
	return bs, nil
}

func newStructEncoder(t reflect.Type, ord binary.AppendByteOrder) (encoderFunc, error) {
	debugEncoder("%s{}", t)
	ret := structEncoder{}
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous || !f.IsExported() {
			continue
		}
		debugEncoder("%s.%s{%s}", t, f.Name, f.Type)
		fEnc, err := typeEncoder(f.Type, ord)
		if err != nil {
			return nil, err
		}
		ret = append(ret, structFieldEncoder{f.Index, fEnc})
	}
	if len(ret) == 0 {
		return nil, unrepresentable(t, "no exported struct fields")
	}
	return ret.encode, nil
}

func newMapEncoder(t reflect.Type, ord binary.AppendByteOrder) (encoderFunc, error) {
	debugEncoder("map[%s]%s{}", t.Key(), t.Elem())
	kt := t.Key()
	switch kt.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16, reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
	default:
		return nil, unrepresentable(t, fmt.Sprintf("unrepresentable map key type %s", kt))
	}
	kEnc, err := typeEncoder(kt, ord)
	if err != nil {
		return nil, err
	}
	vt := t.Elem()
	vEnc, err := typeEncoder(vt, ord)
	if err != nil {
		return nil, err
	}

	return func(bs []byte, v reflect.Value) ([]byte, error) {
		bs = pad(bs, 4)
		ln := v.Len()
		bs = ord.AppendUint32(bs, uint32(ln))
		bs = pad(bs, 8)
		iter := v.MapRange()
		for iter.Next() {
			bs = pad(bs, 8)
			k, v := iter.Key(), iter.Value()

			var err error
			bs, err = kEnc(bs, k)
			if err != nil {
				return nil, err
			}
			bs, err = vEnc(bs, v)
			if err != nil {
				return nil, err
			}
		}
		return bs, nil
	}, nil
}

func pad(bs []byte, align int) []byte {
	extra := len(bs) % align
	if extra == 0 {
		return bs
	}
	var pad [8]byte
	return append(bs, pad[:align-extra]...)
}
