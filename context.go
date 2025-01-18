package dbus

import (
	"context"
	"errors"
	"os"
)

func getCtx[T any](ctx context.Context, key any) (ret T, ok bool) {
	v := ctx.Value(key)
	if v == nil {
		return ret, false
	}
	if ret, ok := v.(T); ok {
		return ret, true
	}
	return ret, false
}

// emitterContextKey is the context key that carries the emitter of a
// DBus signal.
type emitterContextKey struct{}

// withContextEmitter augments ctx with DBus emitter information.
func withContextEmitter(ctx context.Context, iface Interface) context.Context {
	return context.WithValue(ctx, emitterContextKey{}, iface)
}

// ContextEmitter extracts the current DBus emitter information from
// ctx, and reports whether any emitter information was present.
func ContextEmitter(ctx context.Context) (Interface, bool) {
	return getCtx[Interface](ctx, emitterContextKey{})
}

// emitterContextKey is the context key that carries the sender of a
// DBus message.
type senderContextKey struct{}

// withContextSender augments ctx with DBus sender information.
func withContextSender(ctx context.Context, peer Peer) context.Context {
	return context.WithValue(ctx, senderContextKey{}, peer)
}

// ContextSender extracts the current DBus sender information from
// ctx, and reports whether any sender information was present.
func ContextSender(ctx context.Context) (Peer, bool) {
	return getCtx[Peer](ctx, senderContextKey{})
}

// destContextKey is the context key that carries the destination of a
// DBus message.
type destContextKey struct{}

// withContextDest augments ctx with DBus destination information.
func withContextDestination(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, destContextKey{}, name)
}

// ContextSender extracts the current DBus destination information
// from ctx, and reports whether any destination information was
// present.
func ContextDestination(ctx context.Context) (string, bool) {
	return getCtx[string](ctx, destContextKey{})
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
