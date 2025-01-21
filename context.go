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

func withContextHeader(ctx context.Context, conn *Conn, hdr *header) context.Context {
	if hdr.Sender != "" {
		ctx = context.WithValue(ctx, senderContextKey{}, conn.Peer(hdr.Sender))
		if hdr.Type == msgTypeSignal && hdr.Path != "" && hdr.Interface != "" {
			ctx = context.WithValue(ctx, emitterContextKey{}, conn.Peer(hdr.Sender).Object(hdr.Path).Interface(hdr.Interface))
		}
	}
	if hdr.Destination != "" {
		ctx = context.WithValue(ctx, destContextKey{}, conn.Peer(hdr.Destination))
	}
	return ctx
}

// emitterContextKey is the context key that carries the emitter of a
// DBus signal.
type emitterContextKey struct{}

// ContextEmitter extracts the current DBus emitter information from
// ctx, and reports whether any emitter information was present.
func ContextEmitter(ctx context.Context) (Interface, bool) {
	return getCtx[Interface](ctx, emitterContextKey{})
}

// emitterContextKey is the context key that carries the sender of a
// DBus message.
type senderContextKey struct{}

// ContextSender extracts the current DBus sender information from
// ctx, and reports whether any sender information was present.
func ContextSender(ctx context.Context) (Peer, bool) {
	return getCtx[Peer](ctx, senderContextKey{})
}

// destContextKey is the context key that carries the destination of a
// DBus message.
type destContextKey struct{}

// ContextSender extracts the current DBus destination information
// from ctx, and reports whether any destination information was
// present.
func ContextDestination(ctx context.Context) (Peer, bool) {
	return getCtx[Peer](ctx, destContextKey{})
}

// filesContextKey is the context key that carries file descriptors
// received with a DBus message.
type filesContextKey struct{}

// withContextFiles augments ctx with message files.
func withContextFiles(ctx context.Context, files *[]*os.File) context.Context {
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
	fs, ok := v.(*[]*os.File)
	if !ok {
		return nil
	}
	if idx < 0 || int(idx) >= len(*fs) {
		return nil
	}

	return (*fs)[int(idx)]
}

// contextFile adds file to the context's outgoing files buffer.
//
// [File] is the only consumer of this API, being the only way to
// interact with DBus file descriptors.
func contextPutFile(ctx context.Context, file *os.File) (idx uint32, err error) {
	v := ctx.Value(filesContextKey{})
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
