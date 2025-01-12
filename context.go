package dbus

import (
	"context"
	"errors"
	"os"
)

// senderContextKey is the context key that carries the sender of a
// DBus message.
type senderContextKey struct{}

// withContextSender augments ctx with DBus sender information.
func withContextSender(ctx context.Context, iface Interface) context.Context {
	return context.WithValue(ctx, senderContextKey{}, iface)
}

// ContextSender extracts the current DBus sender information from
// ctx, and reports whether any sender information was present.
//
// Sender information is available in [Marshaler] and [Unmarshaler]
// calls.
func ContextSender(ctx context.Context) (Interface, bool) {
	v := ctx.Value(senderContextKey{})
	if v == nil {
		return Interface{}, false
	}
	if ret, ok := v.(Interface); ok {
		return ret, true
	}
	return Interface{}, false
}

// filesContextKey is the context key that carries file descriptors
// received with a DBus message.
type filesContextKey struct{}

// withContextFiles augments ctx with message files.
func withContextFiles(ctx context.Context, files []*os.File) context.Context {
	return context.WithValue(ctx, filesContextKey{}, files)
}

// contextFile returns the idx-th message file in ctx.
//
// [File] is the only consumer of this API, being the only way to
// interact with DBus file descriptors.
func contextFile(ctx context.Context, idx uint32) *os.File {
	v := ctx.Value(filesContextKey{})
	if v == nil {
		return nil
	}
	fs, ok := v.([]*os.File)
	if !ok {
		return nil
	}
	if idx < 0 || int(idx) >= len(fs) {
		return nil
	}

	return fs[int(idx)]
}

// writeFilesContextKey is the context key that carries file
// descriptors to be sent with a DBus message.
type writeFilesContextKey struct{}

// withContextFiles augments ctx with an output slice for files to be
// sent with a message.
func withContextPutFiles(ctx context.Context, files *[]*os.File) context.Context {
	return context.WithValue(ctx, writeFilesContextKey{}, files)
}

// contextFile adds file to the context's outgoing files buffer.
//
// [File] is the only consumer of this API, being the only way to
// interact with DBus file descriptors.
func contextPutFile(ctx context.Context, file *os.File) (idx uint32, err error) {
	v := ctx.Value(writeFilesContextKey{})
	if v == nil {
		return 0, errors.New("cannot send file descriptor: invalid context")
	}
	fsp, ok := v.(*[]*os.File)
	if !ok || fsp == nil {
		return 0, errors.New("cannot send file descriptor: invalid context")
	}

	*fsp = append(*fsp, file)
	return uint32(len(*fsp) - 1), nil
}
