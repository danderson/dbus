package dbus

import (
	"errors"
	"fmt"
	"iter"
	"log"
	"reflect"
	"strings"

	"github.com/danderson/dbus/fragments"
)

// A Signature describes the type of a DBus value.
type Signature struct {
	parts []reflect.Type
}

func mkSignature(parts ...reflect.Type) Signature {
	return Signature{parts}
}

// ParseSignature parses a DBus type signature string.
func ParseSignature(sig string) (Signature, error) {
	var (
		ret  Signature
		rest = sig
		part reflect.Type
		err  error
	)
	for rest != "" {
		part, rest, err = parseOne(rest, false)
		if err != nil {
			return Signature{}, fmt.Errorf("invalid type signature %q: %w", sig, err)
		}
		ret.parts = append(ret.parts, part)
	}
	return ret, nil
}

// parseOne consumes the first complete type from the front of sig,
// and returns the corresponding reflect.Type as well as the remainder
// of the type string.
func parseOne(sig string, inArray bool) (reflect.Type, string, error) {
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

// MustParseSignature parses a DBus signature string into a Signature,
// or panics if the string is invalid.
func MustParseSignature(sig string) Signature {
	ret, err := ParseSignature(sig)
	if err != nil {
		panic(fmt.Sprintf("MustParseSignature(%q): %v", sig, err))
	}
	return ret
}

func (s Signature) String() string {
	switch len(s.parts) {
	case 0:
		return ""
	case 1:
		return signatureStrForType(s.parts[0])
	default:
		ret := make([]string, len(s.parts))
		for i, p := range s.parts {
			ret[i] = signatureStrForType(p)
		}
		return strings.Join(ret, "")
	}
}

func signatureStrForType(t reflect.Type) string {
	if t == reflect.TypeFor[FileDescriptor]() {
		return "h"
	}
	// Check typeToStr first, to convert ObjectPath to its special
	// type rather than lower it to its underlying string.
	if ret := typeToStr[t]; ret != 0 {
		return string(ret)
	}
	if ret := typeToStr[kindToType[t.Kind()]]; ret != 0 {
		return string(ret)
	}

	switch t.Kind() {
	case reflect.Slice:
		return "a" + signatureStrForType(t.Elem())
	case reflect.Map:
		return fmt.Sprintf("a{%s%s}", signatureStrForType(t.Key()), signatureStrForType(t.Elem()))
	case reflect.Struct:
		var ret []string
		for _, f := range reflect.VisibleFields(t) {
			if f.Anonymous || !f.IsExported() {
				continue
			}
			ret = append(ret, signatureStrForType(f.Type))
		}
		return fmt.Sprintf("(%s)", strings.Join(ret, ""))
	default:
		panic(fmt.Sprintf("unknown signature type %s", t))
	}
}

func (s Signature) MarshalDBus(e *fragments.Encoder) error {
	str := s.String()
	if len(str) > 255 {
		return fmt.Errorf("signature exceeds maximum length of 255 bytes")
	}
	e.Uint8(uint8(len(str)))
	e.Write([]byte(str))
	e.Uint8(0)
	return nil
}

func (s *Signature) UnmarshalDBus(st *fragments.Decoder) error {
	u8, err := st.Uint8()
	if err != nil {
		return err
	}
	bs, err := st.Read(int(u8) + 1)
	*s, err = ParseSignature(string(bs[:len(bs)-1]))
	return err
}

func (s Signature) AlignDBus() int { return 1 }

var signatureSignature = mkSignature(reflect.TypeFor[Signature]())

func (s Signature) SignatureDBus() Signature {
	return signatureSignature
}

// IsZero reports whether the signature is the zero value. A zero
// Signature describes a void value.
func (s Signature) IsZero() bool {
	return len(s.parts) == 0
}

// IsSingle reports whether the signature contains a single complete
// type, as opposed to being a multi-type message signature.
func (s Signature) IsSingle() bool {
	return len(s.parts) == 1
}

// onlyType returns s.parts[0] if s.IsSingle(), and panics otherwise.
func (s Signature) onlyType() reflect.Type {
	if !s.IsSingle() {
		panic("onlyType called on non-single signature")
	}
	return s.parts[0]
}

// Parts iterates over the component parts of a DBus type signature.
//
// For signatures representing a single Go type, the iterator yields a
// single value. For type signatures describing a DBus message, the
// iterator yields the Signaturee of each field of the message in
// sequence.
func (s Signature) Parts() iter.Seq[Signature] {
	return func(yield func(Signature) bool) {
		for _, p := range s.parts {
			if !yield(mkSignature(p)) {
				return
			}
		}
	}
}

// Type returns the reflect.Type the Signature represents.
func (s Signature) Type() reflect.Type {
	if s.IsZero() {
		return nil
	}
	if s.IsSingle() {
		return s.parts[0]
	}
	fs := make([]reflect.StructField, len(s.parts))
	for i, p := range s.parts {
		fs[i] = reflect.StructField{
			Name: fmt.Sprintf("Field%d", i),
			Type: p,
		}
	}
	return reflect.StructOf(fs)
}

// Value returns a new reflect.Value for the type the signature
// represents.
func (s Signature) Value() reflect.Value {
	t := s.Type()
	if t == nil {
		return reflect.Value{}
	}
	return reflect.New(t)
}

type signer interface {
	SignatureDBus() Signature
}

var signerType = reflect.TypeFor[signer]()

type sigCacheEntry struct {
	sig Signature
	err error
}

var signatures cache[sigCacheEntry]

func init() {
	// This needs to be an init func to break the initialization cycle
	// between the cache and the calls to the cache within
	// uncachedSignatureOf.
	signatures.Init(
		func(t reflect.Type) sigCacheEntry {
			return sigCacheEntry{uncachedSignatureOf(t), nil}
		},
		func(t reflect.Type) sigCacheEntry {
			sigErr(t, "recursive type")
			panic("unreachable")
		})
}

const debugSignatures = false

func debugSignature(msg string, args ...any) {
	if !debugSignatures {
		return
	}
	log.Printf(msg, args...)
}

// SignatureFor returns the Signature for the given type.
func SignatureFor[T any]() (Signature, error) {
	ret := signatures.GetRecover(reflect.TypeFor[T]())
	if ret.err != nil {
		return Signature{}, ret.err
	}
	return ret.sig, nil
}

// MustSignatureFor is like SignatureFor, but panics if the type has
// no Signature instead of returning an error.
func MustSignatureFor[T any]() Signature {
	ret, err := SignatureFor[T]()
	if err != nil {
		panic(fmt.Sprintf("MustSignatureFor[%s]() failed: %v", reflect.TypeFor[T](), err))
	}
	return ret
}

// SignatureOf returns the Signature for the given value.
func SignatureOf(v any) (Signature, error) {
	ret := signatures.GetRecover(reflect.TypeOf(v))
	if ret.err != nil {
		return Signature{}, ret.err
	}
	return ret.sig, nil
}

func MustSignatureOf(v any) Signature {
	ret, err := SignatureOf(v)
	if err != nil {
		panic(fmt.Sprintf("MustSignatureOf(%s) failed: %v", v, err))
	}
	return ret
}

func sigErr(t reflect.Type, reason string) Signature {
	signatures.Unwind(sigCacheEntry{err: unrepresentable(t, reason)})
	// So that callers can return the result of this constructor and
	// pretend that it's not doing any non-local return. The non-local
	// return is just an optimization so that encoders don't waste
	// time partially encoding types that will never fully succeed.
	return Signature{}
}

func signatureOf(t reflect.Type) Signature {
	debugSignature("signatureOf(%s)", t)
	defer debugSignature("end signatureOf(%s)", t)

	ret := signatures.Get(t)
	if ret.err != nil {
		signatures.Unwind(ret)
	}
	return ret.sig
}

func uncachedSignatureOf(t reflect.Type) Signature {
	debugSignature("uncachedSignatureOf(%s)", t)
	defer debugSignature("end uncachedSignatureOf(%s)", t)
	if t == nil {
		return sigErr(t, "nil interface")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Implements(signerType) {
		return reflect.Zero(t).Interface().(signer).SignatureDBus()
	} else if ptr := reflect.PointerTo(t); ptr.Implements(signerType) {
		return reflect.Zero(ptr).Interface().(signer).SignatureDBus()
	}

	if ret := kindToType[t.Kind()]; ret != nil {
		return mkSignature(ret)
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		es := signatureOf(t.Elem())
		return mkSignature(reflect.SliceOf(es.onlyType()))
	case reflect.Map:
		k := t.Key()
		if k == variantType {
			// Would technically get caught by the struct-ness test
			// below, but Variant is a common dbus thing and we should
			// report a better error for it specifically.
			return sigErr(t, "map keys cannot be Variants")
		}
		switch k.Kind() {
		case reflect.Slice:
			return sigErr(t, "map keys cannot be slices")
		case reflect.Array:
			return sigErr(t, "map keys cannot be arrays")
		case reflect.Struct:
			return sigErr(t, "map keys cannot be structs")
		}
		ks := signatureOf(k)
		vs := signatureOf(t.Elem())

		return mkSignature(reflect.MapOf(ks.onlyType(), vs.onlyType()))
	case reflect.Struct:
		hasFields := false
		for _, f := range reflect.VisibleFields(t) {
			if f.Anonymous || !f.IsExported() {
				continue
			}
			hasFields = true
			// Descend through all the fields, to look for cyclic
			// references.
			signatureOf(f.Type)
		}
		if !hasFields {
			return sigErr(t, "empty struct")
		}
		return mkSignature(t)
	}

	return sigErr(t, "no mapping available")
}
