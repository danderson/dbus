package dbus

import (
	"encoding/binary"
	"errors"
	"os"
)

type FileDescriptor os.File

func (fd *FileDescriptor) MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error) {
	return nil, errors.New("not yet implemented")
}

func (*FileDescriptor) AlignDBus() int           { return 4 }
func (*FileDescriptor) SignatureDBus() Signature { return "h" }
