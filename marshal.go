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
	e := encoderGen{}
	return e.get(t)
}

type encoderGen struct {
	stack []reflect.Type
}

func (e *encoderGen) get(t reflect.Type) (ret fragments.EncoderFunc, err error) {
	if ret, err := encoders.Get(t); err == nil {
		return ret, nil
	} else if !errors.Is(err, errNotFound) {
		return nil, err
	}
	if slices.Contains(e.stack, t) {
		return nil, fmt.Errorf("cannot represent recursive type %s in dbus", t)
	}
	e.stack = append(e.stack, t)

	// Note, defer captures the type value in case it gets messed with
	// below.
	defer func(t reflect.Type) {
		e.stack = e.stack[:len(e.stack)-1]
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
		return e.newCondAddrMarshalEncoder(t), nil
	} else if t.Implements(marshalerType) {
		return e.newMarshalEncoder(), nil
	}

	switch t {
	case reflect.TypeFor[*os.File]():
		return e.newFileEncoder(), nil
	case reflect.TypeFor[ObjectPath]():
		return e.newObjectPathEncoder(), nil
	case reflect.TypeFor[Signature]():
		return e.newSignatureEncoder(), nil
	case reflect.TypeFor[any]():
		return e.newAnyEncoder(), nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		return e.newPtrEncoder(t)
	case reflect.Bool:
		return e.newBoolEncoder(), nil
	case reflect.Int, reflect.Uint:
		return nil, typeErr(t, "int and uint aren't portable, use fixed width integers")
	case reflect.Int8:
		return nil, typeErr(t, "int8 has no corresponding DBus type, use uint8 instead")
	case reflect.Int16, reflect.Int32, reflect.Int64:
		return e.newIntEncoder(t), nil
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.newUintEncoder(t), nil
	case reflect.Float32:
		return nil, typeErr(t, "float32 has no corresponding DBus type, use float64 instead")
	case reflect.Float64:
		return e.newFloatEncoder(), nil
	case reflect.String:
		return e.newStringEncoder(), nil
	case reflect.Slice, reflect.Array:
		return e.newSliceEncoder(t)
	case reflect.Struct:
		return e.newStructEncoder(t)
	case reflect.Map:
		return e.newMapEncoder(t)
	}
	return nil, typeErr(t, "no dbus mapping for type")
}

func (e *encoderGen) newCondAddrMarshalEncoder(t reflect.Type) fragments.EncoderFunc {
	ptr := e.newMarshalEncoder()
	if t.Implements(marshalerType) {
		val := e.newMarshalEncoder()
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

func (e *encoderGen) newMarshalEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		m := v.Interface().(Marshaler)
		return m.MarshalDBus(ctx, e)
	}
}

func (e *encoderGen) newFileEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		f := v.Interface().(*os.File)
		if f == nil {
			return errors.New("cannot marshal nil *os.File")
		}
		idx, err := contextPutFile(ctx, f)
		if err != nil {
			return err
		}
		e.Uint32(idx)
		return nil
	}
}

func (e *encoderGen) newObjectPathEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		p := v.Interface().(ObjectPath)
		e.String(string(p.Clean()))
		return nil
	}
}

func (e *encoderGen) newSignatureEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		s := v.Interface().(Signature).String()
		if len(s) > 255 {
			return fmt.Errorf("signature exceeds maximum length of 255 bytes")
		}
		e.Uint8(uint8(len(s)))
		e.Write([]byte(s))
		e.Uint8(0)
		return nil
	}
}

func (e *encoderGen) newAnyEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		if v.IsNil() {
			return errors.New("cannot marshal nil interface value")
		}
		inner := v.Elem()
		sig, err := SignatureOf(inner.Interface())
		if err != nil {
			return err
		}
		if err := e.Value(ctx, sig); err != nil {
			return err
		}
		if err := e.Value(ctx, inner.Interface()); err != nil {
			return err
		}
		return nil
	}
}

func (e *encoderGen) newPtrEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	elemEnc, err := e.get(t.Elem())
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

func (e *encoderGen) newBoolEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		val := uint32(0)
		if v.Bool() {
			val = 1
		}
		e.Uint32(val)
		return nil
	}
}

func (e *encoderGen) newIntEncoder(t reflect.Type) fragments.EncoderFunc {
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

func (e *encoderGen) newUintEncoder(t reflect.Type) fragments.EncoderFunc {
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

func (e *encoderGen) newFloatEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		e.Uint64(math.Float64bits(v.Float()))
		return nil
	}
}

func (e *encoderGen) newStringEncoder() fragments.EncoderFunc {
	return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
		e.String(v.String())
		return nil
	}
}

func (e *encoderGen) newSliceEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		// Fast path for []byte
		return func(ctx context.Context, e *fragments.Encoder, v reflect.Value) error {
			e.Bytes(v.Bytes())
			return nil
		}, nil
	}

	elemEnc, err := e.get(t.Elem())
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

func (e *encoderGen) newStructEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	fs, err := getStructInfo(t)
	if err != nil {
		return nil, fmt.Errorf("getting struct info for %s: %w", t, err)
	}

	var frags []fragments.EncoderFunc
	for _, f := range fs.StructFields {
		fEnc, err := e.newStructFieldEncoder(f)
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
func (e *encoderGen) newStructFieldEncoder(f *structField) (fragments.EncoderFunc, error) {
	if f.IsVarDict() {
		return e.newVarDictFieldEncoder(f)
	}

	fEnc, err := e.get(f.Type)
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
func (e *encoderGen) newVarDictFieldEncoder(f *structField) (fragments.EncoderFunc, error) {
	kEnc, err := e.get(f.Type.Key())
	if err != nil {
		return nil, err
	}
	vEnc, err := e.get(reflect.TypeFor[any]())
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
					var a any
					va := reflect.ValueOf(&a).Elem()
					va.Set(fv)
					if err := vEnc(ctx, e, va); err != nil {
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

func (e *encoderGen) newMapEncoder(t reflect.Type) (fragments.EncoderFunc, error) {
	kt := t.Key()
	if !mapKeyKinds.Has(kt.Kind()) {
		return nil, typeErr(t, "invalid map key type %s", kt)
	}
	kEnc, err := e.get(kt)
	if err != nil {
		return nil, err
	}
	vt := t.Elem()
	vEnc, err := e.get(vt)
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
