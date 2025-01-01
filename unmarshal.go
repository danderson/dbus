package dbus

import (
	"fmt"
	"io"
	"log"
	"math"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

// Unmarshal parses the DBus wire message data and stores the result
// in the value pointed to by v. If v is nil or not a pointer,
// Unmarshal returns a [TypeError].
//
// Generally, Unmarshal applies the inverse of the rules used by
// [Marshal]. The layout of the wire message must be compatible with
// the target's DBus signature. Since messages generally do not embed
// their signature, it is up to the caller to know the expected
// message format and match it.
//
// Unmarshal traverses the value v recursively. If an encountered value
// implements [Unmarshaler], Unmarshal calls
// [Unmarshaler.UnmarshalDBus] to unmarshal it. Types implementing
// [Unmarshaler] must use a pointer receiver on
// [Unmarshaler.UnmarshalDBus]. Attempting to unmarshal using a value
// receiver UnmarshalDBus method results in a [TypeError].
//
// Otherwise, Unmarshal uses the following type-dependent default
// encodings:
//
// DBus integer, boolean, float and string values decode into the
// corresponding Go types. DBus floats are exclusively double
// precision, but can be decoded into float64, or float32 with a loss
// of precision.
//
// DBus arrays can decode into Go slices or arrays. When decoding into
// an array, the message's array length must match the target array's
// length. When decoding into a slice, Unmarshal resets the slice
// length to zero and then appends each element to the slice.
//
// DBus structs decode into Go structs. Message fields decode into
// struct fields in declaration order, according to the Go struct
// field's type. Embedded struct fields are decoded as if their inner
// exported fields were fields in the outer struct, subject to the
// usual Go visibility rules.
//
// DBus dictionaries decode into Go maps. When decoding into a map,
// Unmarshal first clears the map, or allocates a new one if the
// target map is nil. Then, the dictionary's key-value pairs are
// stored into the map in message order. If the message's map contains
// duplicate entries for a key, all but the last entry's value are
// discarded.
//
// Pointers decode as the value pointed to. Unmarshal allocates zero
// values as needed when it encounters nil pointers.
//
// [Signature], [ObjectPath], and [FileDescriptor] decode the
// corresponding DBus types.
//
// DBus variant values currently cannot be decoded (TODO).
//
// [int], [uint], interface, channel, complex and function values have
// no equivalent DBus type. If Unmarshal encounters such a value it
// will return a [TypeError].
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

// debugDecoders enables spammy debug logging during the construction
// of decoder funcs.
const debugDecoders = false

func debugDecoder(msg string, args ...any) {
	if !debugDecoders {
		return
	}
	log.Printf(msg, args...)
}

// Unmarshaler is the interface implemented by types that can
// unmarshal themselves.
//
// [Unmarshaler.SignatureDBus] and [Unmarshaler.AlignDBus] are called
// with zero receivers, and therefore the returned [Signature] and
// alignment cannot depend on the incoming message.
//
// [Unmarshaler.UnmarshalDBus] must have a pointer receiver. If
// Unmarshal encounters an Unmarshaler whose UnmarshalDBus method
// takes a value receiver, it will return a [TypeError].
//
// [Unmarshaler.UnmarshalDBus] may assume that the output has already
// been padded according to the value returned by
// [Unmarshaler.AlignDBus].
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
	debugDecoder("typeDecoder(%s)", t)
	defer debugDecoder("end typeDecoder(%s)", t)

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
	debugDecoder("%s{} (external Unmarshaler, addressable)", t)
	ptr := newMarshalDecoder(reflect.PointerTo(t))
	return func(st *fragments.Decoder, v reflect.Value) error {
		return ptr(st, v.Addr())
	}
}

func newMarshalDecoder(t reflect.Type) fragments.DecoderFunc {
	debugDecoder("%s{} (external Unmarshaler)", t)
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
	debugDecoder("ptr{%s}", t.Elem())
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
	debugDecoder("bool{}")
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
		debugDecoder("[]byte{}")
		return func(st *fragments.Decoder, v reflect.Value) error {
			bs, err := st.Bytes()
			if err != nil {
				return err
			}
			v.SetBytes(bs)
			return nil
		}
	}

	debugDecoder("[]%s{}", t.Elem())
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

type structFieldDecoder struct {
	idx [][]int
	dec fragments.DecoderFunc
}

type structDecoder []structFieldDecoder

func fieldByIndexAlloc(v reflect.Value, idx [][]int) reflect.Value {
	for i, hop := range idx {
		if i > 0 {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.FieldByIndex(hop)
	}
	return v
}

func (fs structDecoder) decode(st *fragments.Decoder, v reflect.Value) error {
	return st.Struct(func() error {
		for _, f := range fs {
			fv := fieldByIndexAlloc(v, f.idx)
			if err := f.dec(st, fv); err != nil {
				return err
			}
		}
		return nil
	})
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
		frags = append(frags, newStructFieldDecoder(t, f))
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
func newStructFieldDecoder(t reflect.Type, f *structField) fragments.DecoderFunc {
	if f.IsVarDict() {
		return newVarDictFieldDecoder(t, f)
	} else {
		fDec := decoders.Get(f.Type)
		index := allocSteps(t, f.Index)
		return func(d *fragments.Decoder, v reflect.Value) error {
			fv := fieldByIndexAlloc(v, index)
			return fDec(d, fv)
		}
	}
}

// Note, the returned fragment decoder expects to be given the entire
// struct, not just the one field being decoded.
func newVarDictFieldDecoder(t reflect.Type, f *structField) fragments.DecoderFunc {
	kDec := decoders.Get(f.Type.Key())
	vDec := decoders.Get(variantType)

	mapIndex := allocSteps(t, f.Index)
	fields := map[string]*varDictField{}
	allocs := map[string][][]int{}
	for _, key := range f.VarDictFields.MapKeys() {
		vf := f.VarDictField(key)
		fields[vf.StrKey] = vf
		allocs[vf.StrKey] = allocSteps(t, vf.Index)
	}

	return func(d *fragments.Decoder, v reflect.Value) error {
		unknown := fieldByIndexAlloc(v, mapIndex)
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
				fv := fieldByIndexAlloc(v, allocs[keyStr])
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
