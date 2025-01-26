package dbus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"slices"

	"github.com/danderson/dbus/fragments"
)

// Unmarshaler is the interface implemented by types that can
// unmarshal themselves.
//
// SignatureDBus is invoked on a zero value of the Unmarshaler, and
// must return a constant.
//
// UnmarshalDBus must have a pointer receiver. If Unmarshal encounters
// an Unmarshaler whose UnmarshalDBus method takes a value receiver,
// it will return a [TypeError].
//
// UnmarshalDBus is responsible for consuming padding appropriate to
// the values being encoded, and for consuming input in a way that
// agrees with the output of SignatureDBus.
type Unmarshaler interface {
	SignatureDBus() Signature
	UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error
}

var unmarshalerType = reflect.TypeFor[Unmarshaler]()

// unmarshalerOnly is the unmarshal method of Unmarshaler by itself.
//
// It is used to enforce that the unmarshal function is implemented
// with a pointer receiver, without requiring SignatureDBus to also
// have a pointer receiver.
type unmarshalerOnly interface {
	UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error
}

var unmarshalerOnlyType = reflect.TypeFor[unmarshalerOnly]()

var decoders cache[reflect.Type, fragments.DecoderFunc]

// decoderFor returns the decoder func for the given type, if the type
// is representable in the DBus wire format.
func decoderFor(t reflect.Type) (ret fragments.DecoderFunc, err error) {
	d := decoderGen{}
	return d.get(t)
}

type decoderGen struct {
	stack []reflect.Type
}

func (d *decoderGen) get(t reflect.Type) (ret fragments.DecoderFunc, err error) {
	if ret, err := decoders.Get(t); err == nil {
		return ret, nil
	} else if !errors.Is(err, errNotFound) {
		return nil, err
	}

	if slices.Contains(d.stack, t) {
		return nil, fmt.Errorf("cannot represent recursive type %s in dbus", t)
	}
	d.stack = append(d.stack, t)

	// Note, defer captures the type value before we mess with it
	// below.
	defer func(t reflect.Type) {
		d.stack = d.stack[:len(d.stack)-1]
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
			return d.newMarshalDecoder(t), nil
		}
	} else if !isPtr && reflect.PointerTo(t).Implements(unmarshalerType) {
		// Second case, unmarshal into value.
		return d.newAddrMarshalDecoder(t), nil
	}

	switch t {
	case reflect.TypeFor[*os.File]():
		return d.newFileDecoder(), nil
	case reflect.TypeFor[ObjectPath]():
		return d.newObjectPathDecoder(), nil
	case reflect.TypeFor[Signature]():
		return d.newSignatureDecoder(), nil
	case reflect.TypeFor[any]():
		return d.newAnyDecoder(), nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		// Note, pointers to Unmarshaler are handled above.
		return d.newPtrDecoder(t)
	case reflect.Bool:
		return d.newBoolDecoder(), nil
	case reflect.Int, reflect.Uint:
		return nil, typeErr(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8:
		return nil, typeErr(t, "int8 has no corresponding DBus type, use uint8 instead")
	case reflect.Int16, reflect.Int32, reflect.Int64:
		return d.newIntDecoder(t), nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return d.newUintDecoder(t), nil
	case reflect.Float32, reflect.Float64:
		return d.newFloatDecoder(), nil
	case reflect.String:
		return d.newStringDecoder(), nil
	case reflect.Slice, reflect.Array:
		return d.newSliceDecoder(t)
	case reflect.Struct:
		return d.newStructDecoder(t)
	case reflect.Map:
		return d.newMapDecoder(t)
	}

	return nil, typeErr(t, "no dbus mapping for type")
}

func (d *decoderGen) newAddrMarshalDecoder(t reflect.Type) fragments.DecoderFunc {
	ptr := d.newMarshalDecoder(reflect.PointerTo(t))
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		return ptr(ctx, d, v.Addr())
	}
}

func (d *decoderGen) newMarshalDecoder(t reflect.Type) fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			elem := reflect.New(t.Elem())
			v.Set(elem)
		}
		m := v.Interface().(Unmarshaler)
		return m.UnmarshalDBus(ctx, d)
	}
}

func (d *decoderGen) newFileDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		idx, err := d.Uint32()
		if err != nil {
			return err
		}
		file := contextFile(ctx, idx)
		if file == nil {
			return errors.New("cannot unmarshal File: no file descriptor available")
		}
		v.Set(reflect.ValueOf(file))
		return nil
	}
}

func (d *decoderGen) newObjectPathDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		s, err := d.String()
		if err != nil {
			return err
		}
		s = string(ObjectPath(s).Clean())
		v.SetString(s)
		return nil
	}
}

func (d *decoderGen) newSignatureDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		u8, err := d.Uint8()
		if err != nil {
			return err
		}
		bs, err := d.Read(int(u8) + 1)
		sig, err := ParseSignature(string(bs[:len(bs)-1]))
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(sig))
		return nil
	}
}

func (d *decoderGen) newAnyDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		var sig Signature
		if err := d.Value(ctx, &sig); err != nil {
			return fmt.Errorf("reading variant signature: %w", err)
		}
		innerType := sig.Type()
		if innerType == nil {
			return fmt.Errorf("unsupported variant type signature %q", sig)
		}
		inner := reflect.New(innerType)
		if err := d.Value(ctx, inner.Interface()); err != nil {
			return fmt.Errorf("reading variant value (signature %q): %w", sig, err)
		}
		if innerType.Kind() == reflect.Struct {
			v.Set(inner)
		} else {
			v.Set(inner.Elem())
		}
		return nil
	}
}

func (d *decoderGen) newPtrDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	elem := t.Elem()
	elemDec, err := d.get(elem)
	if err != nil {
		return nil, err
	}
	fn := func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			if !v.CanSet() {
				panic("got an unsettable nil pointer, should be impossible!")
			}
			elem := reflect.New(elem)
			if err := elemDec(ctx, d, elem.Elem()); err != nil {
				return err
			}
			v.Set(elem)
		} else if err := elemDec(ctx, d, v.Elem()); err != nil {
			return err
		}
		return nil
	}
	return fn, nil
}

func (d *decoderGen) newBoolDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		u, err := d.Uint32()
		if err != nil {
			return err
		}
		v.SetBool(u != 0)
		return nil
	}
}

func (d *decoderGen) newIntDecoder(t reflect.Type) fragments.DecoderFunc {
	switch t.Size() {
	case 1:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u8, err := d.Uint8()
			if err != nil {
				return err
			}
			v.SetInt(int64(int8(u8)))
			return nil
		}
	case 2:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u16, err := d.Uint16()
			if err != nil {
				return err
			}
			v.SetInt(int64(int16(u16)))
			return nil
		}
	case 4:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u32, err := d.Uint32()
			if err != nil {
				return err
			}
			v.SetInt(int64(int32(u32)))
			return nil
		}
	case 8:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u64, err := d.Uint64()
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

func (d *decoderGen) newUintDecoder(t reflect.Type) fragments.DecoderFunc {
	switch t.Size() {
	case 1:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u8, err := d.Uint8()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u8))
			return nil
		}
	case 2:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u16, err := d.Uint16()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u16))
			return nil
		}
	case 4:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u32, err := d.Uint32()
			if err != nil {
				return err
			}
			v.SetUint(uint64(u32))
			return nil
		}
	case 8:
		return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			u64, err := d.Uint64()
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

func (d *decoderGen) newFloatDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		u64, err := d.Uint64()
		if err != nil {
			return err
		}
		v.SetFloat(math.Float64frombits(u64))
		return nil
	}
}

func (d *decoderGen) newStringDecoder() fragments.DecoderFunc {
	return func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		s, err := d.String()
		if err != nil {
			return err
		}
		v.SetString(s)
		return nil
	}
}

func (d *decoderGen) newSliceDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		fn := func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			bs, err := d.Bytes()
			if err != nil {
				return err
			}
			v.SetBytes(bs)
			return nil
		}
		return fn, nil
	}

	elemDec, err := d.get(t.Elem())
	if err != nil {
		return nil, err
	}
	isStruct := alignAsStruct(t.Elem())

	fn := func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		v.Set(v.Slice(0, 0))

		_, err := d.Array(isStruct, func(i int) error {
			v.Grow(1)
			v.Set(v.Slice(0, i+1))
			if err := elemDec(ctx, d, v.Index(i)); err != nil {
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

func (d *decoderGen) newStructDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	fs, err := getStructInfo(t)
	if err != nil {
		return nil, typeErr(t, "getting struct info: %w", err)
	}

	var frags []fragments.DecoderFunc
	for _, f := range fs.StructFields {
		fDec, err := d.newStructFieldDecoder(f)
		if err != nil {
			return nil, err
		}
		frags = append(frags, fDec)
	}

	var fn fragments.DecoderFunc
	if fs.NoPad {
		fn = func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			for _, frag := range frags {
				if err := frag(ctx, d, v); err != nil {
					return err
				}
			}
			return nil
		}
	} else {
		fn = func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
			return d.Struct(func() error {
				for _, frag := range frags {
					if err := frag(ctx, d, v); err != nil {
						return err
					}
				}
				return nil
			})
		}
	}
	return fn, nil
}

// Note, the returned fragment decoder expects to be given the entire
// struct, not just the one field being decoded.
func (d *decoderGen) newStructFieldDecoder(f *structField) (fragments.DecoderFunc, error) {
	if f.IsVarDict() {
		return d.newVarDictFieldDecoder(f)
	}

	fDec, err := d.get(f.Type)
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
func (d *decoderGen) newVarDictFieldDecoder(f *structField) (fragments.DecoderFunc, error) {
	kDec, err := d.get(f.Type.Key())
	if err != nil {
		return nil, err
	}
	vDec, err := d.get(reflect.TypeFor[any]())
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
		val := reflect.New(reflect.TypeFor[any]())

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
				// TODO: could make the kind test and number of
				// pointer unrolls static, type is known at compile
				// time.
				fv = derefAlloc(fv)
				// *any(underlying) -> underlying
				inner := val.Elem().Elem()
				// the any decoder unmarshals structs as pointers, so
				// need one more indirection.
				if inner.Type().Kind() == reflect.Pointer {
					inner = inner.Elem()
				}
				if fv.Type() != inner.Type() {
					return fmt.Errorf("invalid type %s received for vardict field %s (%s)", inner.Type(), field.Name, fv.Type())
				}
				fv.Set(inner)
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

func (d *decoderGen) newMapDecoder(t reflect.Type) (fragments.DecoderFunc, error) {
	kt := t.Key()
	if !mapKeyKinds.Has(kt.Kind()) {
		return nil, typeErr(t, "invalid map key type %s", kt)
	}
	kDec, err := d.get(kt)
	if err != nil {
		return nil, err
	}
	vt := t.Elem()
	vDec, err := d.get(vt)
	if err != nil {
		return nil, err
	}

	fn := func(ctx context.Context, d *fragments.Decoder, v reflect.Value) error {
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		} else {
			v.Clear()
		}

		key := reflect.New(kt)
		val := reflect.New(vt)

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
