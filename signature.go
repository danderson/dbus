package dbus

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
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

func (s Signature) asStruct() Signature {
	if s.IsZero() {
		return Signature{}
	}
	if s.typ.Kind() == reflect.Struct {
		return s
	}
	ret := reflect.StructOf([]reflect.StructField{
		{
			Name: "Field0",
			Type: s.typ,
		},
	})
	return Signature{ret, "(" + s.str + ")"}
}

// String returns the string encoding of the Signature, as described
// in the DBus specification.
func (s Signature) String() string {
	return s.str
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
	return signatureFor(reflect.TypeFor[T](), nil)
}

// SignatureOf returns the Signature of the given value.
func SignatureOf(v any) (Signature, error) {
	return signatureFor(reflect.TypeOf(v), nil)
}

func signatureFor(t reflect.Type, stack []reflect.Type) (sig Signature, err error) {
	if ret, err := typeToSignature.Get(t); err == nil {
		return ret, nil
	} else if !errors.Is(err, errNotFound) {
		return Signature{}, err
	}

	if slices.Contains(stack, t) {
		return Signature{}, errors.New("recursive type signature")
	}
	stack = append(stack, t)

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

	t = derefType(t)

	if pt := reflect.PointerTo(t); pt.Implements(marshalerType) || pt.Implements(unmarshalerType) {
		if t.Implements(signerType) {
			return reflect.Zero(t).Interface().(signer).SignatureDBus(), nil
		} else {
			return reflect.Zero(pt).Interface().(signer).SignatureDBus(), nil
		}
	}

	switch t {
	case reflect.TypeFor[Signature]():
		return mkSignature(t, "g"), nil
	case reflect.TypeFor[ObjectPath]():
		return mkSignature(t, "o"), nil
	case reflect.TypeFor[os.File]():
		return mkSignature(t, "h"), nil
	case reflect.TypeFor[any]():
		return mkSignature(t, "v"), nil
	}

	if ret := kindToType[t.Kind()]; ret != nil {
		return mkSignature(ret, string(kindToStr[t.Kind()])), nil
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		es, err := signatureFor(t.Elem(), stack)
		if err != nil {
			return Signature{}, err
		}
		return mkSignature(reflect.SliceOf(es.typ), "a"+es.str), nil
	case reflect.Map:
		k := t.Key()
		if k == reflect.TypeFor[any]() {
			return Signature{}, typeErr(t, "map keys cannot be any")
		}
		switch k.Kind() {
		case reflect.Slice:
			return Signature{}, typeErr(t, "map keys cannot be slices")
		case reflect.Array:
			return Signature{}, typeErr(t, "map keys cannot be arrays")
		case reflect.Struct:
			return Signature{}, typeErr(t, "map keys cannot be structs")
		}
		ks, err := signatureFor(k, stack)
		if err != nil {
			return Signature{}, err
		}
		vs, err := signatureFor(t.Elem(), stack)
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
			fieldSig, err := signatureFor(f.Type, stack)
			if err != nil {
				return Signature{}, err
			}
			s = append(s, fieldSig.str)
		}
		if fs.NoPad {
			return mkSignature(t, strings.Join(s, "")), nil
		} else {
			return mkSignature(t, "("+strings.Join(s, "")+")"), nil
		}
	}

	return Signature{}, typeErr(t, "no mapping available")
}
