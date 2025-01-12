package dbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

// unmarshal reads a DBus message from r and stores the result in the
// value pointed to by v. If v is nil or not a pointer, Unmarshal
// returns a [TypeError].
//
// Generally, Unmarshal applies the inverse of the rules used by
// [Marshal]. The layout of the wire message must be compatible with
// the target's DBus signature. Since messages generally do not embed
// their signature, it is up to the caller to know the expected
// message format and match it.
//
// Unmarshal traverses the value v recursively. If an encountered
// value implements [Unmarshaler], Unmarshal calls UnmarshalDBus to
// unmarshal it. Types implementing [Unmarshaler] must implement
// UnmarshalDBus with a pointer receiver. Attempting to unmarshal
// using an UnmarshalDBus method with a value receiver results in a
// [TypeError].
//
// Otherwise, Unmarshal uses the following type-dependent default
// encodings:
//
// uint{8,16,32,64}, int{16,32,64}, float64, bool and string values
// encode the corresponding DBus basic types.
//
// Array and slice values decode DBus arrays. When decoding into an
// array, the message's array length must match the target array's
// length. When decoding into a slice, Unmarshal resets the slice
// length to zero and then appends each element to the slice.
//
// Struct values decode DBus structs. The message's fields decode into
// the target struct's fields in declaration order. Embedded struct
// fields are decoded as if their inner exported fields were fields in
// the outer struct, subject to the usual Go visibility rules.
//
// Maps decode DBus dictionaries. When decoding into a map, Unmarshal
// first clears the map, or allocates a new one if the target map is
// nil. Then, the incoming key-value pairs are stored into the map in
// message order. If the incoming dictionary contains duplicate values
// for a key, all but the last value are discarded.
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
// A vardict field decodes a DBus dictionary just like regular map,
// except that if an incoming key matches an associated field's tag,
// the corresponding value decodes into that associated field instead,
// with the [Variant] envelope removed. If the associated field's type
// is incompatible with the received map value, Unmarshal returns a
// [TypeError].
//
// Pointers decode as the value pointed to. Unmarshal allocates zero
// values as needed when it encounters nil pointers.
//
// [Signature], [ObjectPath], and [File] decode the corresponding DBus
// types.
//
// [Variant] values decode DBus variants. The type of the variant's
// inner value is determined by the type signature carried in the
// message. Variants containing a struct are decoded into an anonymous
// struct with fields named Field0, Field1, ..., FieldN in message
// order.
//
// int8, int, uint, uintptr, complex64, complex128, interface,
// channel, and function values cannot decode any DBus type.
// Attempting to decode such values causes Unmarshal to return a
// [TypeError].
//
// DBus cannot represent cyclic or recursive types. Attempting to
// decode into such values causes Unmarshal to return a
// [TypeError].
func unmarshal(ctx context.Context, data io.Reader, ord fragments.ByteOrder, v any) error {
	if v == nil {
		return fmt.Errorf("can't unmarshal into nil interface")
	}
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer {
		return fmt.Errorf("can't unmarshal into a non-pointer")
	}
	if val.IsNil() {
		return fmt.Errorf("can't unmarshal into a nil pointer")
	}
	dec, err := decoderFor(val.Type().Elem())
	if err != nil {
		return err
	}
	st := fragments.Decoder{
		Order:  ord,
		Mapper: decoderFor,
		In:     data,
	}
	return dec(ctx, &st, val.Elem())
}

// Unmarshaler is the interface implemented by types that can
// unmarshal themselves.
//
// SignatureDBus and IsDBusStruct are invoked on zero values of the
// Unmarshaler, and must return constant values.
//
// UnmarshalDBus must have a pointer receiver. If Unmarshal encounters
// an Unmarshaler whose UnmarshalDBus method takes a value receiver,
// it will return a [TypeError].
//
// UnmarshalDBus is responsible for consuming padding appropriate to
// the values being encoded, and for consuming input in a way that
// agrees with the values of SignatureDBus and IsDBusStruct.
type Unmarshaler interface {
	SignatureDBus() Signature
	IsDBusStruct() bool
	UnmarshalDBus(ctx context.Context, st *fragments.Decoder) error
}

var unmarshalerType = reflect.TypeFor[Unmarshaler]()

// unmarshalerOnly is the unmarshal method of Unmarshaler by itself.
//
// It is used to enforce that the unmarshal function is implemented
// with a pointer receiver, without requiring that SignatureDBus and
// IsDBusStruct also have a pointer receiver.
type unmarshalerOnly interface {
	UnmarshalDBus(ctx context.Context, st *fragments.Decoder) error
}

var unmarshalerOnlyType = reflect.TypeFor[unmarshalerOnly]()

var decoders cache[reflect.Type, fragments.DecoderFunc]

// decoderFor returns the decoder func for the given type, if the type
// is representable in the DBus wire format.
func decoderFor(t reflect.Type) (ret fragments.DecoderFunc, err error) {
	if ret, err := decoders.Get(t); err == nil {
		return ret, nil
	} else if !errors.Is(err, errNotFound) {
		return nil, err
	}
	// Note, defer captures the type value before we mess with it
	// below.
	defer func(t reflect.Type) {
		if err != nil {
			decoders.SetErr(t, err)
		} else {
			decoders.Set(t, ret)
		}
	}(t)

	// We only want Unmarshalers with pointer receivers, since a value
	// receiver would silently discard the results of the
	// UnmarshalDBus call and lead to confusing bugs. There are two
	// cases we need to look for.
	//
	// The first is a pointer that implements Unmarshaler, and whose
	// pointed-to type does not implement Unmarshaler. This means the
	// type implements Unmarshaler with pointer receivers, and we can
	// call it.
	//
	// The second is a value that does not implement Unmarshaler, but
	// whose pointer does. In that case, we can take the value's
	// address and use the pointer unmarshaler. Unmarshal only hands
	// us values that are addressable, so we don't need an
	// addressability check to do this.
	isPtr := t.Kind() == reflect.Pointer
	if t.Implements(unmarshalerType) {
		if !isPtr || t.Elem().Implements(unmarshalerOnlyType) {
			return nil, typeErr(t, "refusing to use dbus.Unmarshaler implementation with value receiver, Unmarshalers must use pointer receivers.")
		} else {
			// First case, can unmarshal into pointer.
			return newMarshalDecoder(t), nil
		}
	} else if !isPtr && reflect.PointerTo(t).Implements(unmarshalerType) {
		// Second case, unmarshal into value.
		return newAddrMarshalDecoder(t), nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		// Note, pointers to Unmarshaler are handled above.
		return newPtrDecoder(t)
	case reflect.Bool:
		return newBoolDecoder(), nil
	case reflect.Int, reflect.Uint:
		return nil, typeErr(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8:
		return nil, typeErr(t, "int8 has no corresponding DBus type, use uint8 instead")
	case reflect.Int16, reflect.Int32, reflect.Int64:
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

	return nil, typeErr(t, "no dbus mapping for type")
}

func newAddrMarshalDecoder(t reflect.Type) fragments.DecoderFunc {
	ptr := newMarshalDecoder(reflect.PointerTo(t))
	return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
		return ptr(ctx, st, v.Addr())
	}
}

func newMarshalDecoder(t reflect.Type) fragments.DecoderFunc {
	return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			elem := reflect.New(t.Elem())
			v.Set(elem)
		}
		m := v.Interface().(Unmarshaler)
		return m.UnmarshalDBus(ctx, st)
	}
}

func newPtrDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	elem := t.Elem()
	elemDec, err := decoderFor(elem)
	if err != nil {
		return nil, err
	}
	fn := func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			if !v.CanSet() {
				panic("got an unsettable nil pointer, should be impossible!")
			}
			elem := reflect.New(elem)
			if err := elemDec(ctx, st, elem.Elem()); err != nil {
				return err
			}
			v.Set(elem)
		} else if err := elemDec(ctx, st, v.Elem()); err != nil {
			return err
		}
		return nil
	}
	return fn, nil
}

func newBoolDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
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
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
			u8, err := st.Uint8()
			if err != nil {
				return err
			}
			v.SetInt(int64(int8(u8)))
			return nil
		}
	case 2:
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
			u16, err := st.Uint16()
			if err != nil {
				return err
			}
			v.SetInt(int64(int16(u16)))
			return nil
		}
	case 4:
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
			u32, err := st.Uint32()
			if err != nil {
				return err
			}
			v.SetInt(int64(int32(u32)))
			return nil
		}
	case 8:
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
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
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
			u8, err := st.Uint8()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u8))
			return nil
		}
	case 2:
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
			u16, err := st.Uint16()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u16))
			return nil
		}
	case 4:
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
			u32, err := st.Uint32()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u32))
			return nil
		}
	case 8:
		return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
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
	return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
		u64, err := st.Uint64()
		if err != nil {
			return err
		}
		v.SetFloat(math.Float64frombits(u64))
		return nil
	}
}

func newStringDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
		s, err := st.String()
		if err != nil {
			return err
		}
		v.SetString(s)
		return nil
	}
}

func newSliceDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		fn := func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
			bs, err := st.Bytes()
			if err != nil {
				return err
			}
			v.SetBytes(bs)
			return nil
		}
		return fn, nil
	}

	elemDec, err := decoderFor(t.Elem())
	if err != nil {
		return nil, err
	}
	isStruct := alignAsStruct(t.Elem())

	fn := func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
		v.Set(v.Slice(0, 0))

		_, err := st.Array(isStruct, func(i int) error {
			v.Grow(1)
			v.Set(v.Slice(0, i+1))
			if err := elemDec(ctx, st, v.Index(i)); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	}
	return fn, nil
}

func newStructDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	fs, err := getStructInfo(t)
	if err != nil {
		return nil, typeErr(t, "getting struct info: %w", err)
	}

	var frags []fragments.DecoderFunc
	for _, f := range fs.StructFields {
		fDec, err := newStructFieldDecoder(f)
		if err != nil {
			return nil, err
		}
		frags = append(frags, fDec)
	}

	fn := func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		return d.Struct(func() error {
			for _, frag := range frags {
				if err := frag(ctx, d, v); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return fn, nil
}

// Note, the returned fragment decoder expects to be given the entire
// struct, not just the one field being decoded.
func newStructFieldDecoder(f *structField) (fragments.DecoderFunc, error) {
	if f.IsVarDict() {
		return newVarDictFieldDecoder(f)
	}

	fDec, err := decoderFor(f.Type)
	if err != nil {
		return nil, err
	}
	fn := func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		fv := f.GetWithAlloc(v)
		return fDec(ctx, d, fv)
	}
	return fn, nil
}

// Note, the returned fragment decoder expects to be given the entire
// struct, not just the one field being decoded.
func newVarDictFieldDecoder(f *structField) (fragments.DecoderFunc, error) {
	kDec, err := decoderFor(f.Type.Key())
	if err != nil {
		return nil, err
	}
	vDec, err := decoderFor(variantType)
	if err != nil {
		return nil, err
	}

	fields := map[string]*varDictField{}
	for _, key := range f.VarDictFields.MapKeys() {
		vf := f.VarDictField(key)
		fields[vf.StrKey] = vf
	}

	fn := func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		unknown := f.GetWithAlloc(v)
		unknownInit := false

		key := reflect.New(f.Type.Key())
		val := reflect.New(variantType)

		_, err := d.Array(true, func(i int) error {
			key.Elem().SetZero()
			val.Elem().SetZero()

			err := d.Struct(func() error {
				if err := kDec(ctx, d, key.Elem()); err != nil {
					return err
				}
				if err := vDec(ctx, d, val.Elem()); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}

			keyStr := fmt.Sprint(key.Elem())
			if field := fields[keyStr]; field != nil {
				fv := field.GetWithAlloc(v)
				inner := val.Elem().Interface().(Variant).Value
				innerVal := reflect.ValueOf(inner)
				if fv.Type() != innerVal.Type() {
					return fmt.Errorf("invalid type %s received for vardict field %s (%s)", innerVal.Type(), field.Name, fv.Type())
				}
				fv.Set(innerVal)
			} else {
				if !unknownInit {
					unknownInit = true
					if unknown.IsNil() {
						unknown.Set(reflect.MakeMap(unknown.Type()))
					} else {
						unknown.Clear()
					}
				}
				unknown.SetMapIndex(key.Elem(), val.Elem())
			}

			return nil
		})
		return err
	}
	return fn, nil
}

func newMapDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	kt := t.Key()
	if !mapKeyKinds.Has(kt.Kind()) {
		return nil, typeErr(t, "invalid map key type %s", kt)
	}
	kDec, err := decoderFor(kt)
	if err != nil {
		return nil, err
	}
	vt := t.Elem()
	vDec, err := decoderFor(vt)
	if err != nil {
		return nil, err
	}

	fn := func(ctx context.Context, st *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		} else {
			v.Clear()
		}

		key := reflect.New(kt)
		val := reflect.New(vt)

		_, err := st.Array(true, func(i int) error {
			key.Elem().SetZero()
			val.Elem().SetZero()
			err := st.Struct(func() error {
				if err := kDec(ctx, st, key.Elem()); err != nil {
					return err
				}
				if err := vDec(ctx, st, val.Elem()); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
			v.SetMapIndex(key.Elem(), val.Elem())
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	}
	return fn, nil
}
