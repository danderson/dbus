package dbus

import (
	"encoding/binary"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
)

type Signature string

func (s Signature) MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error) {
	if len(s) > 255 {
		return nil, fmt.Errorf("signature exceeds maximum length of 255 bytes")
	}
	bs = append(bs, byte(len(s)))
	bs = append(bs, s...)
	bs = append(bs, 0)
	return bs, nil
}

func (s Signature) AlignDBus() int           { return 1 }
func (s Signature) SignatureDBus() Signature { return "g" }

type signer interface {
	SignatureDBus() Signature
}

var signerType = reflect.TypeFor[signer]()

var sigCache sync.Map

const debugSignatures = true

func debugSignature(msg string, args ...any) {
	if !debugSignatures {
		return
	}
	log.Printf(msg, args...)
}

func SignatureOf(v any) (Signature, error) {
	return signatureOf(reflect.TypeOf(v))
}

func signatureOf(t reflect.Type) (sig Signature, err error) {
	debugSignature("signatureOf(%s)", t)
	defer debugSignature("end signatureOf(%s)", t)

	if cached, loaded := sigCache.LoadOrStore(t, nil); loaded {
		debugSignature("signatureOf(%s) cache = %v", t, cached)
		if cached == nil {
			err := unrepresentable(t, "recursive type")
			sigCache.CompareAndSwap(t, nil, err)
			return "", err
		}
		if err, ok := cached.(error); ok {
			return "", err
		}
		return cached.(Signature), nil
	}
	// The defer captures t, so that the dereference loop further down
	// doesn't result in us caching the result to the wrong place.
	defer func(t reflect.Type) {
		debugSignature("signatureOf(%s) = %s, %v", t, sig, err)
		if err != nil {
			sigCache.CompareAndSwap(t, nil, err)
		} else {
			sigCache.CompareAndSwap(t, nil, sig)
		}
	}(t)

	if t == nil {
		return "", unrepresentable(t, "nil interface")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Implements(signerType) {
		log.Printf("XXXXXX value impl signer")
		return reflect.Zero(t).Interface().(signer).SignatureDBus(), nil
	} else if ptr := reflect.PointerTo(t); ptr.Implements(signerType) {
		log.Printf("XXXXXX ptr impl signer")
		return reflect.Zero(ptr).Interface().(signer).SignatureDBus(), nil
	}

	switch t.Kind() {
	case reflect.Bool:
		return "b", nil
	case reflect.Int8, reflect.Uint8:
		return "y", nil
	case reflect.Int16:
		return "n", nil
	case reflect.Uint16:
		return "q", nil
	case reflect.Int32:
		return "i", nil
	case reflect.Uint32:
		return "u", nil
	case reflect.Int64:
		return "x", nil
	case reflect.Uint64:
		return "t", nil
	case reflect.Float32, reflect.Float64:
		return "d", nil
	case reflect.String:
		return "s", nil
	case reflect.Slice, reflect.Array:
		e := t.Elem()
		es, err := signatureOf(e)
		if err != nil {
			return "", err
		}
		return "a" + es, nil
	case reflect.Map:
		k := t.Key()
		if k == variantType {
			// Would technically get caught by the struct-ness test
			// below, but Variant is a common dbus thing and we should
			// report a better error for it specifically.
			return "", unrepresentable(t, "map keys cannot be Variants")
		}
		switch k.Kind() {
		case reflect.Slice:
			return "", unrepresentable(t, "map keys cannot be slices")
		case reflect.Array:
			return "", unrepresentable(t, "map keys cannot be arrays")
		case reflect.Struct:
			return "", unrepresentable(t, "map keys cannot be structs")
		}
		ks, err := signatureOf(k)
		if err != nil {
			return "", err
		}

		v := t.Elem()
		vs, err := signatureOf(v)
		if err != nil {
			return "", err
		}

		return Signature(fmt.Sprintf("a{%s%s}", ks, vs)), nil
	case reflect.Struct:
		var ret []string
		for _, f := range reflect.VisibleFields(t) {
			if f.Anonymous || !f.IsExported() {
				continue
			}
			s, err := signatureOf(f.Type)
			if err != nil {
				return "", err
			}
			ret = append(ret, string(s))
		}
		if len(ret) == 0 {
			return "", unrepresentable(t, "empty struct")
		}
		return Signature(fmt.Sprintf("(%s)", strings.Join(ret, ""))), nil
	}

	return "", unrepresentable(t, "no mapping available")
}
