package dbus

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/danderson/dbus/fragments"
)

type Signature string

func (s Signature) MarshalDBus(st *fragments.Encoder) error {
	if len(s) > 255 {
		return fmt.Errorf("signature exceeds maximum length of 255 bytes")
	}
	st.Uint8(uint8(len(s)))
	st.Write([]byte(s))
	st.Uint8(0)
	return nil
}

func (s *Signature) UnmarshalDBus(st *fragments.Decoder) error {
	u8, err := st.Uint8()
	if err != nil {
		return err
	}
	bs, err := st.Read(int(u8) + 1)
	*s = Signature(bs[:len(bs)-1])
	return nil
}

func (s Signature) AlignDBus() int           { return 1 }
func (s Signature) SignatureDBus() Signature { return "g" }

func (s Signature) Type() reflect.Type {
	if s == "" {
		return nil
	}
	switch s[0] {
	case '(':
		return nil // TODO: support dynamic structs
	case '{':
		return nil // TODO: support dict entries
	case 'b':
		return reflect.TypeFor[bool]()
	case 'y':
		return reflect.TypeFor[uint8]()
	case 'n':
		return reflect.TypeFor[int16]()
	case 'q':
		return reflect.TypeFor[uint16]()
	case 'i':
		return reflect.TypeFor[int32]()
	case 'u':
		return reflect.TypeFor[uint32]()
	case 'x':
		return reflect.TypeFor[int64]()
	case 't':
		return reflect.TypeFor[uint64]()
	case 'd':
		return reflect.TypeFor[float64]()
	case 's':
		return reflect.TypeFor[string]()
	case 'v':
		return reflect.TypeFor[Variant]()
	case 'a':
		elem := s[1:].Type()
		if elem == nil {
			return nil
		}
		return reflect.SliceOf(elem)
	default:
		return nil
	}
}

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

func SignatureOf(v any) (Signature, error) {
	ret := signatures.GetRecover(reflect.TypeOf(v))
	if ret.err != nil {
		return "", ret.err
	}
	return ret.sig, nil
}

func sigErr(t reflect.Type, reason string) Signature {
	signatures.Unwind(sigCacheEntry{err: unrepresentable(t, reason)})
	// So that callers can return the result of this constructor and
	// pretend that it's not doing any non-local return. The non-local
	// return is just an optimization so that encoders don't waste
	// time partially encoding types that will never fully succeed.
	return ""
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

	switch t.Kind() {
	case reflect.Bool:
		return "b"
	case reflect.Int8, reflect.Uint8:
		return "y"
	case reflect.Int16:
		return "n"
	case reflect.Uint16:
		return "q"
	case reflect.Int32:
		return "i"
	case reflect.Uint32:
		return "u"
	case reflect.Int64:
		return "x"
	case reflect.Uint64:
		return "t"
	case reflect.Float32, reflect.Float64:
		return "d"
	case reflect.String:
		return "s"
	case reflect.Slice, reflect.Array:
		es := signatureOf(t.Elem())
		return "a" + es
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

		return Signature(fmt.Sprintf("a{%s%s}", ks, vs))
	case reflect.Struct:
		var ret []string
		for _, f := range reflect.VisibleFields(t) {
			if f.Anonymous || !f.IsExported() {
				continue
			}
			s := signatureOf(f.Type)
			ret = append(ret, string(s))
		}
		if len(ret) == 0 {
			return sigErr(t, "empty struct")
		}
		return Signature(fmt.Sprintf("(%s)", strings.Join(ret, "")))
	}

	return sigErr(t, "no mapping available")
}
