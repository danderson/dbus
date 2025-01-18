package dbus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/danderson/dbus/fragments"
)

// marshal returns the DBus wire encoding of v, using the given byte
// ordering.
//
// Marshal traverses the value v recursively. If an encountered value
// implements [Marshaler], Marshal calls MarshalDBus on it to produce
// its encoding.
//
// Otherwise, Marshal uses the following type-dependent default
// encodings:
//
// uint{8,16,32,64}, int{16,32,64}, float64, bool and string values
// encode to the corresponding DBus basic type.
//
// Array and slice values encode as DBus arrays. Nil slices encode the
// same as an empty slice.
//
// Struct values encode as DBus structs. Each exported struct field is
// encoded in declaration order, according to its own type. Embedded
// struct fields are encoded as if their inner exported fields were
// fields in the outer struct, subject to the usual Go visibility
// rules.
//
// Map values encode as a DBus dictionary, i.e. an array of key/value
// pairs. The map's key underlying type must be uint{8,16,32,64},
// int{16,32,64}, float64, bool, or string.
//
// Several DBus protocols use map[K]dbus.Variant values to extend
// structs with new fields in a backwards compatible way. To support
// this "vardict" idiom, structs may contain a single "vardict" field
// and several "associated" fields:
//
//	struct Vardict{
//	    // A "vardict" map for the struct.
//	    M map[uint8]dbus.Variant `dbus:"vardict"`
//
//	    // "associated" fields. Associated fields can be declared
//	    // anywhere in the struct, before or after the vardict field.
//	    Foo string `dbus:"key=1"`
//	    Bar uint32 `dbus:"key=2"`
//	}
//
// A vardict field encodes as a DBus dictionary just like a regular
// map, except that associated fields with nonzero values are encoded
// as additional key/value pairs. An associated field can be tagged
// with `dbus:"key=X,encodeZero"` to encode its zero value as well.
//
// Pointer values encode as the value pointed to. A nil pointer
// encodes as the zero value of the type pointed to.
//
// [Signature], [ObjectPath], and [File] values encode to the
// corresponding DBus types.
//
// [Variant] values encode as DBus variants. The Variant's inner value
// must be a valid value according to these rules, or Marshal will
// return a [TypeError].
//
// int8, int, uint, uintptr, complex64, complex128, interface,
// channel, and function values cannot be encoded. Attempting to
// encode such values causes Marshal to return a [TypeError].
//
// DBus cannot represent cyclic or recursive types. Attempting to
// encode such values causes Marshal to return a [TypeError].
func marshal(ctx context.Context, v any, ord fragments.ByteOrder) ([]byte, error) {
	val := reflect.ValueOf(v)
	enc, err := encoderFor(val.Type())
	if err != nil {
		return nil, err
	}
	e := fragments.Encoder{
		Order:  ord,
		Mapper: encoderFor,
	}
	if err := enc(ctx, &e, val); err != nil {
		return nil, err
	}
	return e.Out, nil
}

// Marshaler is the interface implemented by types that can marshal
// themselves to the DBus wire format.
//
// SignatureDBus and IsDBusStruct are invoked on zero values of the
// Marshaler, and must return constant values.
//
// MarshalDBus is responsible for inserting padding appropriate to the
// values being encoded, and for producing output that matches the
// structure declared by SignatureDBus and IsDBusStruct.
type Marshaler interface {
	SignatureDBus() Signature
	IsDBusStruct() bool
	MarshalDBus(ctx context.Context, e *fragments.Encoder) error
}

var marshalerType = reflect.TypeFor[Marshaler]()

var encoders cache[reflect.Type, fragments.EncoderFunc]

func encoderFor(t reflect.Type) (ret fragments.EncoderFunc, err error) {
	if ret, err := encoders.Get(t); err == nil {
		return ret, nil
	} else if !errors.Is(err, errNotFound) {
		return nil, err
	}
	// Note, defer captures the type value in case it gets messed with
	// below.
	defer func(t reflect.Type) {
		if err != nil {
			encoders.SetErr(t, err)
		} else {
			encoders.Set(t, ret)
		}
	}(t)

	// If a value's pointer type implements Unmarshaler, we can avoid
	// a value copy by using it. But we can only use it for
	// addressable values, which requires an additional runtime check.
	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(marshalerType) {
		return newCondAddrMarshalEncoder(t), nil
	} else if t.Implements(marshalerType) {
		return newMarshalEncoder(), nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		return newPtrEncoder(t)
	case reflect.Bool:
		return newBoolEncoder(), nil
	case reflect.Int, reflect.Uint:
		return nil, typeErr(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8:
		return nil, typeErr(t, "int8 has no corresponding DBus type, use uint8 instead")
	case reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntEncoder(t), nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newUintEncoder(t), nil
	case reflect.Float32:
		return nil, typeErr(t, "float32 has no corresponding DBus type, use float64 instead")
	case reflect.Float64:
		return newFloatEncoder(), nil
	case reflect.String:
		return newStringEncoder(), nil
	case reflect.Slice, reflect.Array:
		return newSliceEncoder(t)
	case reflect.Struct:
		return newStructEncoder(t)
	case reflect.Map:
		return newMapEncoder(t)
	}
	return nil, typeErr(t, "no dbus mapping for type")
}

func newCondAddrMarshalEncoder(t reflect.Type) fragments.EncoderFunc {
	ptr := newMarshalEncoder()
	if t.Implements(marshalerType) {
		val := newMarshalEncoder()
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			if v.CanAddr() {
				return ptr(ctx, e, v.Addr())
			} else {
				return val(ctx, e, v)
			}
		}
	} else {
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			if !v.CanAddr() {
				return typeErr(t, "Marshaler is only implemented on pointer receiver, and cannot take the address of given value")
			}
			return ptr(ctx, e, v.Addr())
		}
	}
}

func newMarshalEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		m := v.Interface().(Marshaler)
		return m.MarshalDBus(ctx, e)
	}
}

func newPtrEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	elemEnc, err := encoderFor(t.Elem())
	if err != nil {
		return nil, err
	}
	fn := func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		if v.IsNil() {
			return elemEnc(ctx, e, reflect.Zero(t))
		}
		return elemEnc(ctx, e, v.Elem())
	}
	return fn, nil
}

func newBoolEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		val := uint32(0)
		if v.Bool() {
			val = 1
		}
		e.Uint32(val)
		return nil
	}
}

func newIntEncoder(t reflect.Type) fragments.EncoderFunc {
	switch t.Size() {
	case 2:
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Uint16(uint16(v.Int()))
			return nil
		}
	case 4:
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Uint32(uint32(v.Int()))
			return nil
		}
	case 8:
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Uint64(uint64(v.Int()))
			return nil
		}
	default:
		panic("invalid newIntEncoder type")
	}
}

func newUintEncoder(t reflect.Type) fragments.EncoderFunc {
	switch t.Size() {
	case 1:
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Uint8(uint8(v.Uint()))
			return nil
		}
	case 2:
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Uint16(uint16(v.Uint()))
			return nil
		}
	case 4:
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Uint32(uint32(v.Uint()))
			return nil
		}
	case 8:
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Uint64(v.Uint())
			return nil
		}
	default:
		panic("invalid newIntEncoder type")
	}
}

func newFloatEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		e.Uint64(math.Float64bits(v.Float()))
		return nil
	}
}

func newStringEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		e.String(v.String())
		return nil
	}
}

func newSliceEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		// Fast path for []byte
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Bytes(v.Bytes())
			return nil
		}, nil
	}

	elemEnc, err := encoderFor(t.Elem())
	if err != nil {
		return nil, err
	}
	isStruct := alignAsStruct(t.Elem())

	fn := func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		return e.Array(isStruct, func() error {
			for i := 0; i < v.Len(); i++ {
				if err := elemEnc(ctx, e, v.Index(i)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return fn, nil
}

func newStructEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	fs, err := getStructInfo(t)
	if err != nil {
		return nil, fmt.Errorf("getting struct info for %s: %w", t, err)
	}

	var frags []fragments.EncoderFunc
	for _, f := range fs.StructFields {
		fEnc, err := newStructFieldEncoder(f)
		if err != nil {
			return nil, err
		}
		frags = append(frags, fEnc)
	}

	fn := func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		e.Struct(func() error {
			for _, frag := range frags {
				if err := frag(ctx, e, v); err != nil {
					return err
				}
			}
			return nil
		})
		return nil
	}
	return fn, nil
}

// Note, the returned fragment encoder expects to be given the entire
// struct, not just the one field being encoded.
func newStructFieldEncoder(f *structField) (fragments.EncoderFunc, error) {
	if f.IsVarDict() {
		return newVarDictFieldEncoder(f)
	}

	fEnc, err := encoderFor(f.Type)
	if err != nil {
		return nil, err
	}
	fn := func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		fv := f.GetWithZero(v)
		return fEnc(ctx, e, fv)
	}
	return fn, nil
}

// Note, the returned fragment encoder expects to be given the entire
// struct, not just the one field being encoded.
func newVarDictFieldEncoder(f *structField) (fragments.EncoderFunc, error) {
	kEnc, err := encoderFor(f.Type.Key())
	if err != nil {
		return nil, err
	}
	vEnc, err := encoderFor(variantType)
	if err != nil {
		return nil, err
	}
	kCmp := f.VarDictKeyCmp()

	fieldKeys := f.VarDictFields.MapKeys()
	slices.SortFunc(fieldKeys, kCmp)
	var varDictFields []*varDictField
	for _, k := range fieldKeys {
		varDictFields = append(varDictFields, f.VarDictField(k))
	}

	fn := func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		return e.Array(true, func() error {
			for _, f := range varDictFields {
				fv := f.GetWithZero(v)
				if fv.IsZero() && !f.EncodeZero {
					continue
				}

				err := e.Struct(func() error {
					if err := kEnc(ctx, e, f.Key); err != nil {
						return err
					}
					if err := vEnc(ctx, e, reflect.ValueOf(Variant{fv.Interface()})); err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					return err
				}
			}

			other := f.GetWithZero(v)
			ks := other.MapKeys()
			slices.SortFunc(ks, kCmp)
			for _, mapKey := range ks {
				mapVal := other.MapIndex(mapKey)
				err := e.Struct(func() error {
					if err := kEnc(ctx, e, mapKey); err != nil {
						return err
					}
					if err := vEnc(ctx, e, mapVal); err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					return err
				}
			}

			return nil
		})
	}
	return fn, nil
}

func newMapEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	kt := t.Key()
	if !mapKeyKinds.Has(kt.Kind()) {
		return nil, typeErr(t, "invalid map key type %s", kt)
	}
	kEnc, err := encoderFor(kt)
	if err != nil {
		return nil, err
	}
	vt := t.Elem()
	vEnc, err := encoderFor(vt)
	if err != nil {
		return nil, err
	}
	kCmp := mapKeyCmp(kt)

	fn := func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		ks := v.MapKeys()
		slices.SortFunc(ks, kCmp)
		return e.Array(true, func() error {
			for _, mk := range ks {
				mv := v.MapIndex(mk)
				err := e.Struct(func() error {
					if err := kEnc(ctx, e, mk); err != nil {
						return err
					}
					if err := vEnc(ctx, e, mv); err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
	return fn, nil
}
