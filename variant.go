package dbus

import (
	"reflect"

	"github.com/danderson/dbus/fragments"
)

type Variant struct {
	Value any
}

var variantType = reflect.TypeFor[Variant]()

func (v Variant) MarshalDBus(st *fragments.Encoder) error {
	sig, err := SignatureOf(v.Value)
	if err != nil {
		return err
	}
	if err := st.Value(sig); err != nil {
		return err
	}
	if err := st.Value(v.Value); err != nil {
		return err
	}
	return nil
}

func (v Variant) AlignDBus() int           { return 1 }
func (v Variant) SignatureDBus() Signature { return "v" }
