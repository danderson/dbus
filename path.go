package dbus

import (
	"reflect"

	"github.com/danderson/dbus/fragments"
)

type ObjectPath string

func (p ObjectPath) MarshalDBus(st *fragments.Encoder) error {
	st.Value(string(p))
	return nil
}

func (p *ObjectPath) UnmarshalDBus(st *fragments.Decoder) error {
	var s string
	if err := st.Value(&s); err != nil {
		return err
	}
	*p = ObjectPath(s)
	return nil
}

func (p ObjectPath) AlignDBus() int { return 4 }

var objectPathSignature = mkSignature(reflect.TypeFor[ObjectPath]())

func (p ObjectPath) SignatureDBus() Signature { return objectPathSignature }
