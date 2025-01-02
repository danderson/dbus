package dbus

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/danderson/dbus/fragments"
)

// Marshal returns the DBus wire encoding of v, using the given byte
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
func Marshal(ctx context.Context, v any, ord fragments.ByteOrder) ([]byte, error) {
	val := reflect.ValueOf(v)
	enc := encoders.GetRecover(val.Type())
	st := fragments.Encoder{
		Order:  ord,
		Mapper: encoders.GetRecover,
	}
	if err := enc(ctx, &st, val); err != nil {
		return nil, err
	}
	return st.Out, nil
}

// Marshaler is the interface implemented by types that can marshal
// themselves to the DBus wire format.
//
// SignatureDBus and AlignDBus must return constants that do not
// depend on the value being encoded.
//
// MarshalDBus may assume that the output has already been padded
// according to the value returned by AlignDBus.
type Marshaler interface {
	SignatureDBus() Signature
	AlignDBus() int
	MarshalDBus(ctx context.Context, st *fragments.Encoder) error
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

// uncachedTypeEncoder returns the EncoderFunc for t.
func uncachedTypeEncoder(t reflect.Type) (ret fragments.EncoderFunc) {
	// If a value's pointer type implements Unmarshaler, we can avoid
	// a value copy by using it. But we can only use it for
	// addressable values, which requires an additional runtime check.
	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(marshalerType) {
		return newCondAddrMarshalEncoder(t)
	} else if t.Implements(marshalerType) {
		return newMarshalEncoder()
	}

	switch t.Kind() {
	case reflect.Pointer:
		return newPtrEncoder(t)
	case reflect.Bool:
		return newBoolEncoder()
	case reflect.Int, reflect.Uint:
		return newErrEncoder(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8:
		return newErrEncoder(t, "int8 has no corresponding DBus type, use uint8 instead")
	case reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntEncoder(t)
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newUintEncoder(t)
	case reflect.Float32:
		return newErrEncoder(t, "float32 has no corresponding DBus type, use float64 instead")
	case reflect.Float64:
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

// newErrEncoder signals that the requested type cannot be encoded to
// the DBus wire format for the given reason.
//
// Internally the function triggers an unwind back to Marshal, caching
// the error with all intermediate EncoderFuncs. This saves time
// during decoding because Marshal can return an error immediately,
// rather than get halfway through a complex object only to discover
// that it cannot be encoded.
//
// However, the semantics are equivalent to returning the error
// encoder normally, so callers may use this function like any other
// EncoderFunc constructor.
func newErrEncoder(t reflect.Type, reason string) fragments.EncoderFunc {
	err := typeErr(t, reason)
	encoders.Unwind(func(context.Context, *fragments.Encoder, reflect.Value) error {
		return err
	})
	// So that callers can return the result of this constructor and
	// pretend that it's not doing any non-local return. The non-local
	// return is just an optimization so that encoders don't waste
	// time partially encoding types that will never fully succeed.
	return nil
}

func newCondAddrMarshalEncoder(t reflect.Type) fragments.EncoderFunc {
	ptr := newMarshalEncoder()
	if t.Implements(marshalerType) {
		val := newMarshalEncoder()
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			if v.CanAddr() {
				return ptr(ctx, st, v.Addr())
			} else {
				return val(ctx, st, v)
			}
		}
	} else {
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			if !v.CanAddr() {
				return typeErr(t, "Marshaler is only implemented on pointer receiver, and cannot take the address of given value")
			}
			return ptr(ctx, st, v.Addr())
		}
	}
}

func newMarshalEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
		m := v.Interface().(Marshaler)
		st.Pad(m.AlignDBus())
		return m.MarshalDBus(ctx, st)
	}
}

func newPtrEncoder(t reflect.Type) fragments.EncoderFunc {
	elemEnc := encoders.Get(t.Elem())
	return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
		if v.IsNil() {
			return elemEnc(ctx, st, reflect.Zero(t))
		}
		return elemEnc(ctx, st, v.Elem())
	}
}

func newBoolEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
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
	case 2:
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			st.Uint16(uint16(v.Int()))
			return nil
		}
	case 4:
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			st.Uint32(uint32(v.Int()))
			return nil
		}
	case 8:
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
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
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			st.Uint8(uint8(v.Uint()))
			return nil
		}
	case 2:
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			st.Uint16(uint16(v.Uint()))
			return nil
		}
	case 4:
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			st.Uint32(uint32(v.Uint()))
			return nil
		}
	case 8:
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			st.Uint64(v.Uint())
			return nil
		}
	default:
		panic("invalid newIntEncoder type")
	}
}

func newFloatEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
		st.Uint64(math.Float64bits(v.Float()))
		return nil
	}
}

func newStringEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
		st.String(v.String())
		return nil
	}
}

func newSliceEncoder(t reflect.Type) fragments.EncoderFunc {
	if t.Elem().Kind() == reflect.Uint8 {
		// Fast path for []byte
		return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
			st.Bytes(v.Bytes())
			return nil
		}
	}

	elemEnc := encoders.Get(t.Elem())
	var isStruct bool
	if t.Elem().Implements(marshalerType) {
		isStruct = reflect.Zero(t.Elem()).Interface().(Marshaler).AlignDBus() == 8
	} else if ptr := reflect.PointerTo(t.Elem()); ptr.Implements(marshalerType) {
		isStruct = reflect.Zero(ptr).Interface().(Marshaler).AlignDBus() == 8
	} else {
		isStruct = t.Elem().Kind() == reflect.Struct
	}

	return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
		return st.Array(isStruct, func() error {
			for i := 0; i < v.Len(); i++ {
				if err := elemEnc(ctx, st, v.Index(i)); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func newStructEncoder(t reflect.Type) fragments.EncoderFunc {
	fs, err := getStructInfo(t)
	if err != nil {
		return newErrEncoder(t, err.Error())
	}
	if len(fs.StructFields) == 0 {
		return newErrEncoder(t, "no exported struct fields")
	}

	var frags []fragments.EncoderFunc
	for _, f := range fs.StructFields {
		frags = append(frags, newStructFieldEncoder(f))
	}

	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
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
}

// Note, the returned fragment encoder expects to be given the entire
// struct, not just the one field being encoded.
func newStructFieldEncoder(f *structField) fragments.EncoderFunc {
	if f.IsVarDict() {
		return newVarDictFieldEncoder(f)
	} else {
		fEnc := encoders.Get(f.Type)
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			fv := f.GetWithZero(v)
			return fEnc(ctx, e, fv)
		}
	}
}

// Note, the returned fragment encoder expects to be given the entire
// struct, not just the one field being encoded.
func newVarDictFieldEncoder(f *structField) fragments.EncoderFunc {
	kEnc := encoders.Get(f.Type.Key())
	vEnc := encoders.Get(variantType)
	kCmp := f.VarDictKeyCmp()

	fieldKeys := f.VarDictFields.MapKeys()
	slices.SortFunc(fieldKeys, kCmp)
	var varDictFields []*varDictField
	for _, k := range fieldKeys {
		varDictFields = append(varDictFields, f.VarDictField(k))
	}

	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
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
}

func newMapEncoder(t reflect.Type) fragments.EncoderFunc {
	kt := t.Key()
	if !mapKeyKinds.Has(kt.Kind()) {
		return newErrEncoder(t, fmt.Sprintf("invalid map key type %s", kt))
	}
	kEnc := encoders.Get(kt)
	vt := t.Elem()
	vEnc := encoders.Get(vt)
	kCmp := mapKeyCmp(kt)

	return func(ctx context.Context, st *fragments.Encoder, v reflect.Value) error {
		ks := v.MapKeys()
		slices.SortFunc(ks, kCmp)
		return st.Array(true, func() error {
			for _, mk := range ks {
				mv := v.MapIndex(mk)
				err := st.Struct(func() error {
					if err := kEnc(ctx, st, mk); err != nil {
						return err
					}
					if err := vEnc(ctx, st, mv); err != nil {
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
}
