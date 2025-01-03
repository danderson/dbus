package dbus

import (
	"context"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

type ObjectPath string

func (p ObjectPath) MarshalDBus(ctx context.Context, st *fragments.Encoder) error {
	st.Value(ctx, string(p))
	return nil
}

func (p *ObjectPath) UnmarshalDBus(ctx context.Context, st *fragments.Decoder) error {
	var s string
	if err := st.Value(ctx, &s); err != nil {
		return err
	}
	*p = ObjectPath(s)
	return nil
}

func (p ObjectPath) IsDBusStruct() bool { return false }

var objectPathSignature = mkSignature(reflect.TypeFor[ObjectPath]())

func (p ObjectPath) SignatureDBus() Signature { return objectPathSignature }
