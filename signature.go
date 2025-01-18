package dbus

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/danderson/dbus/fragments"
)

// A Signature describes the type of a DBus value.
type Signature struct {
	typ reflect.Type
	str string
}

func (s Signature) asMsgBody() Signature {
	if s.typ.Kind() != reflect.Struct {
		return s
	}
	return Signature{s.typ, s.str[1 : len(s.str)-1]}
}

// String returns the string encoding of the Signature, as described
// in the DBus specification.
func (s Signature) String() string {
	return s.str
}

func (s Signature) MarshalDBus(ctx context.Context, e *fragments.Encoder) error {
	if len(s.str) > 255 {
		return fmt.Errorf("signature exceeds maximum length of 255 bytes")
	}
	e.Uint8(uint8(len(s.str)))
	e.Write([]byte(s.str))
	e.Uint8(0)
	return nil
}

func (s *Signature) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	u8, err := d.Uint8()
	if err != nil {
		return err
	}
	bs, err := d.Read(int(u8) + 1)
	*s, err = ParseSignature(string(bs[:len(bs)-1]))
	return err
}

func (s Signature) IsDBusStruct() bool { return false }

var signatureSignature = mkSignature(reflect.TypeFor[Signature](), "g")

func (s Signature) SignatureDBus() Signature {
	return signatureSignature
}

// IsZero reports whether the signature is the zero value. A zero
// Signature describes a void value.
func (s Signature) IsZero() bool {
	return s.typ == nil
}

// Type returns the reflect.Type the Signature represents.
//
// If [Signature.IsZero] is true, Type returns nil.
func (s Signature) Type() reflect.Type {
	return s.typ
}

var (
	typeToSignature cache[reflect.Type, Signature]
	strToSignature  cache[string, Signature]
)

func mkSignature(typ reflect.Type, str string) Signature {
	return Signature{typ, str}
}

// ParseSignature parses a DBus type signature string.
func ParseSignature(sig string) (Signature, error) {
	if ret, err := strToSignature.Get(sig); err == nil {
		return ret, nil
	} else if !errors.Is(err, errNotFound) {
		return Signature{}, err
	}

	var (
		rest  = sig
		parts []reflect.Type
		part  reflect.Type
		err   error
	)
	for rest != "" {
		part, rest, err = parseOne(rest, false)
		if err != nil {
			err := fmt.Errorf("invalid type signature %q: %w", sig, err)
			strToSignature.SetErr(sig, err)
			return Signature{}, err
		}
		parts = append(parts, part)
	}

	var ret Signature
	switch len(parts) {
	case 0:
		ret = mkSignature(nil, "")
	case 1:
		ret = mkSignature(parts[0], sig)
	default:
		fs := make([]reflect.StructField, len(parts))
		for i, f := range parts {
			fs[i] = reflect.StructField{
				Name: fmt.Sprintf("Field%d", i),
				Type: f,
			}
		}
		st := reflect.StructOf(fs)
		ret = mkSignature(st, "("+sig+")")
		// Also add the adjusted struct signature to cache.
		strToSignature.Set(ret.str, ret)
	}

	typeToSignature.Set(ret.typ, ret)
	strToSignature.Set(sig, ret)

	return ret, nil
}

func mustParseSignature(sig string) Signature {
	ret, err := ParseSignature(sig)
	if err != nil {
		panic(err)
	}
	return ret
}

// parseOne consumes the first complete type from the front of sig,
// and returns the corresponding reflect.Type as well as the remainder
// of the type string.
func parseOne(sig string, inArray bool) (t reflect.Type, rest string, err error) {
	if ret, ok := strToType[sig[0]]; ok {
		return ret, sig[1:], nil
	}

	switch sig[0] {
	case 'a':
		isDict := len(sig) > 1 && sig[1] == '{'
		elem, rest, err := parseOne(sig[1:], true)
		if err != nil {
			return nil, "", err
		}
		if isDict {
			return elem, rest, nil // sub-parser already produced a map
		}
		return reflect.SliceOf(elem), rest, nil
	case '(':
		var (
			fields []reflect.Type
			field  reflect.Type
			rest   = sig[1:]
			err    error
		)
		for rest != "" && rest[0] != ')' {
			field, rest, err = parseOne(rest, false)
			if err != nil {
				return nil, "", err
			}
			fields = append(fields, field)
		}
		if rest == "" {
			return nil, "", fmt.Errorf("missing closing ) in struct definition")
		}
		fs := make([]reflect.StructField, len(fields))
		for i, f := range fields {
			fs[i] = reflect.StructField{
				Name: fmt.Sprintf("Field%d", i),
				Type: f,
			}
		}
		return reflect.StructOf(fs), rest[1:], nil
	case '{':
		if !inArray {
			return nil, "", errors.New("dict entry type found outside array")
		}
		key, rest, err := parseOne(sig[1:], false)
		if err != nil {
			return nil, "", err
		}
		if !mapKeyKinds.Has(key.Kind()) {
			return nil, "", fmt.Errorf("invalid dict entry key type %s, must be a dbus basic type", key)
		}
		val, rest, err := parseOne(rest, false)
		if err != nil {
			return nil, "", err
		}
		if rest == "" || rest[0] != '}' {
			return nil, "", errors.New("missing closing } in dict entry definition")
		}
		return reflect.MapOf(key, val), rest[1:], nil
	default:
		return nil, "", fmt.Errorf("unknown type specifier %q", sig[0])
	}
}

// A signer provides its own DBus signature.
type signer interface {
	SignatureDBus() Signature
}

var signerType = reflect.TypeFor[signer]()

// SignatureFor returns the Signature for the given type.
func SignatureFor[T any]() (Signature, error) {
	return signatureFor(reflect.TypeFor[T]())
}

// SignatureOf returns the Signature of the given value.
func SignatureOf(v any) (Signature, error) {
	return signatureFor(reflect.TypeOf(v))
}

func signatureFor(t reflect.Type) (sig Signature, err error) {
	if ret, err := typeToSignature.Get(t); err == nil {
		return ret, nil
	} else if !errors.Is(err, errNotFound) {
		return Signature{}, err
	}
	// Note, defer captures the type value before we mess with it
	// below.
	defer func(t reflect.Type) {
		if err != nil {
			typeToSignature.SetErr(t, err)
		} else {
			typeToSignature.Set(t, sig)
		}
	}(t)

	if t == nil {
		return Signature{}, typeErr(t, "nil interface")
	}

	// Deref all but one level of pointers, to check for Marshaler/Unmarshaler.
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	t = reflect.PointerTo(t)

	if t.Implements(marshalerType) || t.Implements(unmarshalerType) {
		if t.Elem().Implements(signerType) {
			return reflect.Zero(t.Elem()).Interface().(signer).SignatureDBus(), nil
		} else {
			return reflect.Zero(t).Interface().(signer).SignatureDBus(), nil
		}
	}

	// Strip off the last pointer layer, the rest of the signature
	// logic operates on the leaf type.
	t = t.Elem()

	if ret := kindToType[t.Kind()]; ret != nil {
		return mkSignature(ret, string(kindToStr[t.Kind()])), nil
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		es, err := signatureFor(t.Elem())
		if err != nil {
			return Signature{}, err
		}
		return mkSignature(reflect.SliceOf(es.typ), "a"+es.str), nil
	case reflect.Map:
		k := t.Key()
		if k == variantType {
			// Would technically get caught by the struct-ness test
			// below, but Variant is a common dbus thing and we should
			// report a better error for it specifically.
			return Signature{}, typeErr(t, "map keys cannot be Variants")
		}
		switch k.Kind() {
		case reflect.Slice:
			return Signature{}, typeErr(t, "map keys cannot be slices")
		case reflect.Array:
			return Signature{}, typeErr(t, "map keys cannot be arrays")
		case reflect.Struct:
			return Signature{}, typeErr(t, "map keys cannot be structs")
		}
		ks, err := signatureFor(k)
		if err != nil {
			return Signature{}, err
		}
		vs, err := signatureFor(t.Elem())
		if err != nil {
			return Signature{}, err
		}

		return mkSignature(reflect.MapOf(ks.typ, vs.typ), "a{"+ks.str+vs.str+"}"), nil
	case reflect.Struct:
		fs, err := getStructInfo(t)
		if err != nil {
			return Signature{}, typeErr(t, "getting struct info: %w", err)
		}
		var s []string
		for _, f := range fs.StructFields {
			// Descend through all fields, to look for cyclic
			// references.
			fieldSig, err := signatureFor(f.Type)
			if err != nil {
				return Signature{}, err
			}
			s = append(s, fieldSig.str)
		}
		return mkSignature(t, "("+strings.Join(s, "")+")"), nil
	}

	return Signature{}, typeErr(t, "no mapping available")
}
