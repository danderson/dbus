package dbus

import "github.com/danderson/dbus/fragments"

type ObjectPath string

func (o ObjectPath) MarshalDBus(st *fragments.Encoder) error {
	st.Value(string(o))
	return nil
}

func (o *ObjectPath) UnmarshalDBus(st *fragments.Decoder) error {
	var s string
	if err := st.Value(&s); err != nil {
		return err
	}
	*o = ObjectPath(s)
	return nil
}

func (ObjectPath) AlignDBus() int           { return 4 }
func (ObjectPath) SignatureDBus() Signature { return "o" }
