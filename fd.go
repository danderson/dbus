package dbus

import (
	"encoding/binary"
	"errors"
	"os"
	"reflect"
)

type FileDescriptor os.File

func (fd *FileDescriptor) MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error) {
	return nil, errors.New("not yet implemented")
}

func (*FileDescriptor) AlignDBus() int { return 4 }

var fdSignature = mkSignature(reflect.TypeFor[*FileDescriptor]())

func (*FileDescriptor) SignatureDBus() Signature { return fdSignature }
