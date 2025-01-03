package dbus

import (
	"context"
	"path"
	"reflect"
	"strings"

	"github.com/danderson/dbus/fragments"
)

type ObjectPath string

func (p ObjectPath) MarshalDBus(ctx context.Context, st *fragments.Encoder) error {
	st.Value(ctx, string(p.Clean()))
	return nil
}

func (p *ObjectPath) UnmarshalDBus(ctx context.Context, st *fragments.Decoder) error {
	var s string
	if err := st.Value(ctx, &s); err != nil {
		return err
	}
	*p = ObjectPath(s).Clean()
	return nil
}

func (p ObjectPath) IsDBusStruct() bool { return false }

var objectPathSignature = mkSignature(reflect.TypeFor[ObjectPath]())

func (p ObjectPath) SignatureDBus() Signature { return objectPathSignature }

func (p ObjectPath) Clean() ObjectPath {
	return ObjectPath(path.Clean(string(p)))
}

func (p ObjectPath) String() string {
	return string(p.Clean())
}

func (p ObjectPath) Valid() bool {
	return path.IsAbs(string(p.Clean()))
}

func (p ObjectPath) IsChildOf(parent ObjectPath) bool {
	sparent := string(parent.Clean())
	sp := string(p.Clean())
	if len(sp) <= len(sparent) {
		return false
	}
	return strings.HasPrefix(sp, sparent+"/")
}
