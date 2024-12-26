package dbus

import (
	"encoding/binary"
	"reflect"
)

type Variant struct {
	Value any
}

var variantType = reflect.TypeFor[Variant]()

func (v Variant) MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error) {
	sig, err := SignatureOf(v.Value)
	if err != nil {
		return nil, err
	}
	bs, err = MarshalAppend(bs, sig, ord)
	if err != nil {
		return nil, err
	}
	return MarshalAppend(bs, v.Value, ord)
}

func (v Variant) AlignDBus() int           { return 1 }
func (v Variant) SignatureDBus() Signature { return "v" }
