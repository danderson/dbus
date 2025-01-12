package dbus

import (
	"context"
	"errors"
	"os"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

// File is a file to be sent or received over the bus.
type File struct {
	*os.File
}

func (f *File) IsDBusStruct() bool { return false }

var fdSignature = mkSignature(reflect.TypeFor[File](), "h")

func (f *File) SignatureDBus() Signature { return fdSignature }

func (f *File) MarshalDBus(ctx context.Context, e *fragments.Encoder) error {
	if f.File == nil {
		return errors.New("cannot marshal File: File.File is nil")
	}
	idx, err := contextPutFile(ctx, f.File)
	if err != nil {
		return err
	}
	e.Uint32(idx)
	return nil
}

func (f *File) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	idx, err := d.Uint32()
	if err != nil {
		return err
	}
	file := contextFile(ctx, idx)
	if file == nil {
		return errors.New("cannot unmarshal File: no file descriptor available")
	}
	f.File = file
	return nil
}
