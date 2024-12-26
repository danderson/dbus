package dbus

import "encoding/binary"

type ObjectPath string

func (o ObjectPath) MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error) {
	return MarshalAppend(bs, string(o), ord)
}

func (ObjectPath) AlignDBus() int           { return 4 }
func (ObjectPath) SignatureDBus() Signature { return "o" }
