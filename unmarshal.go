package dbus

import (
	"fmt"
	"io"
	"math"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

// Unmarshal reads a DBus message from r and stores the result in the
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
// Pointers decode as the value pointed to. Unmarshal allocates zero
// values as needed when it encounters nil pointers.
//
// [Signature], [ObjectPath], and [FileDescriptor] decode the
// corresponding DBus types.
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
func Unmarshal(data io.Reader, ord fragments.ByteOrder, v any) error {
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
	dec := decoders.GetRecover(val.Type().Elem())
	st := fragments.Decoder{
		Order:  ord,
		Mapper: decoders.GetRecover,
		In:     data,
	}
	return dec(&st, val.Elem())
}

// Unmarshaler is the interface implemented by types that can
// unmarshal themselves.
//
// SignatureDBus and AlignDBus must return constants that do not
// depend on the value being decoded.
//
// UnmarshalDBus must have a pointer receiver. If Unmarshal encounters
// an Unmarshaler whose UnmarshalDBus method takes a value receiver,
// it will return a [TypeError].
//
// UnmarshalDBus may assume that the output has already been padded
// according to the value returned by AlignDBus.
type Unmarshaler interface {
	SignatureDBus() Signature
	AlignDBus() int
	UnmarshalDBus(st *fragments.Decoder) error
}

var unmarshalerType = reflect.TypeFor[Unmarshaler]()

type unmarshalerOnly interface {
	UnmarshalDBus(st *fragments.Decoder) error
}

var unmarshalerOnlyType = reflect.TypeFor[unmarshalerOnly]()

var decoders cache[fragments.DecoderFunc]

func init() {
	// This needs to be an init func to break the initialization cycle
	// between the cache and the calls to the cache within
	// uncachedTypeEncoder.
	decoders.Init(uncachedTypeDecoder, func(t reflect.Type) fragments.DecoderFunc {
		return newErrDecoder(t, "recursive type")
	})
}

// uncachedTypeDecoder returns the DecoderFunc for t.
func uncachedTypeDecoder(t reflect.Type) fragments.DecoderFunc {
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
			return newErrDecoder(t, "refusing to use dbus.Unmarshaler implementation with value receivers, Unmarshalers must use pointer receivers.")
		} else {
			// First case, can unmarshal into pointer.
			return newMarshalDecoder(t)
		}
	} else if !isPtr && reflect.PointerTo(t).Implements(unmarshalerType) {
		// Second case, unmarshal into value.
		return newAddrMarshalDecoder(t)
	}

	switch t.Kind() {
	case reflect.Pointer:
		// Note, pointers to Unmarshaler are handled above.
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

// newErrDecoder signals that the requested type cannot be decoded to
// the DBus wire format for the given reason.
//
// Internally the function triggers an unwind back to Unmarshal,
// caching the error with all intermediate DecoderFuncs. This saves
// time during decoding because Unmarshal can return an error
// immediately, rather than get halfway through a complex object only
// to discover that it cannot be decoded.
//
// However, the semantics are equivalent to returning the error
// decoder normally, so callers may use this function like any other
// DecoderFunc constructor.
func newErrDecoder(t reflect.Type, reason string) fragments.DecoderFunc {
	err := typeErr(t, reason)
	decoders.Unwind(func(*fragments.Decoder, reflect.Value) error {
		return err
	})
	// So that callers can return the result of this constructor and
	// pretend that it's not doing any non-local return. The non-local
	// return is just an optimization so that decoders don't waste
	// time partially decoding types that will never fully succeed.
	return nil
}

func newAddrMarshalDecoder(t reflect.Type) fragments.DecoderFunc {
	ptr := newMarshalDecoder(reflect.PointerTo(t))
	return func(st *fragments.Decoder, v reflect.Value) error {
		return ptr(st, v.Addr())
	}
}

func newMarshalDecoder(t reflect.Type) fragments.DecoderFunc {
	return func(st *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			elem := reflect.New(t.Elem())
			v.Set(elem)
		}
		m := v.Interface().(Unmarshaler)
		st.Pad(m.AlignDBus())
		return m.UnmarshalDBus(st)
	}
}

func newPtrDecoder(t reflect.Type) fragments.DecoderFunc {
	elem := t.Elem()
	elemDec := decoders.Get(elem)
	return func(st *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			if !v.CanSet() {
				panic("got an unsettable nil pointer, should be impossible!")
			}
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
	return func(st *fragments.Decoder, v reflect.Value) error {
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
		return func(st *fragments.Decoder, v reflect.Value) error {
			u8, err := st.Uint8()
			if err != nil {
				return err
			}
			v.SetInt(int64(int8(u8)))
			return nil
		}
	case 2:
		return func(st *fragments.Decoder, v reflect.Value) error {
			u16, err := st.Uint16()
			if err != nil {
				return err
			}
			v.SetInt(int64(int16(u16)))
			return nil
		}
	case 4:
		return func(st *fragments.Decoder, v reflect.Value) error {
			u32, err := st.Uint32()
			if err != nil {
				return err
			}
			v.SetInt(int64(int32(u32)))
			return nil
		}
	case 8:
		return func(st *fragments.Decoder, v reflect.Value) error {
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
		return func(st *fragments.Decoder, v reflect.Value) error {
			u8, err := st.Uint8()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u8))
			return nil
		}
	case 2:
		return func(st *fragments.Decoder, v reflect.Value) error {
			u16, err := st.Uint16()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u16))
			return nil
		}
	case 4:
		return func(st *fragments.Decoder, v reflect.Value) error {
			u32, err := st.Uint32()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u32))
			return nil
		}
	case 8:
		return func(st *fragments.Decoder, v reflect.Value) error {
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
	return func(st *fragments.Decoder, v reflect.Value) error {
		u64, err := st.Uint64()
		if err != nil {
			return err
		}
		v.SetFloat(math.Float64frombits(u64))
		return nil
	}
}

func newStringDecoder() fragments.DecoderFunc {
	return func(st *fragments.Decoder, v reflect.Value) error {
		s, err := st.String()
		if err != nil {
			return err
		}
		v.SetString(s)
		return nil
	}
}

func newSliceDecoder(t reflect.Type) fragments.DecoderFunc {
	if t.Elem().Kind() == reflect.Uint8 {
		return func(st *fragments.Decoder, v reflect.Value) error {
			bs, err := st.Bytes()
			if err != nil {
				return err
			}
			v.SetBytes(bs)
			return nil
		}
	}

	elemDec := decoders.Get(t.Elem())
	var isStruct bool
	if t.Elem().Implements(unmarshalerType) {
		isStruct = reflect.Zero(t.Elem()).Interface().(Marshaler).AlignDBus() == 8
	} else if ptr := reflect.PointerTo(t.Elem()); ptr.Implements(marshalerType) {
		isStruct = reflect.Zero(ptr).Interface().(Marshaler).AlignDBus() == 8
	} else {
		isStruct = t.Elem().Kind() == reflect.Struct
	}

	return func(st *fragments.Decoder, v reflect.Value) error {
		v.Set(v.Slice(0, 0))

		_, err := st.Array(isStruct, func(i int) error {
			v.Grow(1)
			v.Set(v.Slice(0, i+1))
			if err := elemDec(st, v.Index(i)); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func newStructDecoder(t reflect.Type) fragments.DecoderFunc {
	fs, err := getStructInfo(t)
	if err != nil {
		return newErrDecoder(t, err.Error())
	}
	if len(fs.StructFields) == 0 {
		return newErrDecoder(t, "no exported struct fields")
	}

	var frags []fragments.DecoderFunc
	for _, f := range fs.StructFields {
		frags = append(frags, newStructFieldDecoder(f))
	}

	return func(d *fragments.Decoder, v reflect.Value) error {
		return d.Struct(func() error {
			for _, frag := range frags {
				if err := frag(d, v); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

// Note, the returned fragment decoder expects to be given the entire
// struct, not just the one field being decoded.
func newStructFieldDecoder(f *structField) fragments.DecoderFunc {
	if f.IsVarDict() {
		return newVarDictFieldDecoder(f)
	} else {
		fDec := decoders.Get(f.Type)
		return func(d *fragments.Decoder, v reflect.Value) error {
			fv := f.GetWithAlloc(v)
			return fDec(d, fv)
		}
	}
}

// Note, the returned fragment decoder expects to be given the entire
// struct, not just the one field being decoded.
func newVarDictFieldDecoder(f *structField) fragments.DecoderFunc {
	kDec := decoders.Get(f.Type.Key())
	vDec := decoders.Get(variantType)

	fields := map[string]*varDictField{}
	for _, key := range f.VarDictFields.MapKeys() {
		vf := f.VarDictField(key)
		fields[vf.StrKey] = vf
	}

	return func(d *fragments.Decoder, v reflect.Value) error {
		unknown := f.GetWithAlloc(v)
		unknownInit := false

		key := reflect.New(f.Type.Key())
		val := reflect.New(variantType)

		_, err := d.Array(true, func(i int) error {
			key.Elem().SetZero()
			val.Elem().SetZero()

			err := d.Struct(func() error {
				if err := kDec(d, key.Elem()); err != nil {
					return err
				}
				if err := vDec(d, val.Elem()); err != nil {
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
}

func newMapDecoder(t reflect.Type) fragments.DecoderFunc {
	kt := t.Key()
	if !mapKeyKinds.Has(kt.Kind()) {
		return newErrDecoder(t, fmt.Sprintf("invalid map key type %s", kt))
	}
	kDec := decoders.Get(kt)
	vt := t.Elem()
	vDec := decoders.Get(vt)

	return func(st *fragments.Decoder, v reflect.Value) error {
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
				if err := kDec(st, key.Elem()); err != nil {
					return err
				}
				if err := vDec(st, val.Elem()); err != nil {
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
}
