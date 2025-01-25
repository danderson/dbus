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

// ContextEmitter returns the emitter value from ctx, and reports
// whether an emitter was found.
//
// Emitter information is available in [Unmarshaler]'s UnmarshalDBus
// method, when decoding a signal type.
func ContextEmitter(ctx context.Context) (Interface, bool) {
	return getCtx[Interface](ctx, emitterContextKey{})
}

// senderContextKey is the context key that carries the sender of a
// DBus message.
type senderContextKey struct{}

// ContextSender returns the sender found in ctx, and reports whether
// a sender was found.
//
// Sender information is available in [Unmarshaler]'s UnmarshalDBus
// method when decoding method arguments and signal types, and in
// method handlers when handling incoming method calls.
func ContextSender(ctx context.Context) (Peer, bool) {
	return getCtx[Peer](ctx, senderContextKey{})
}

// destContextKey is the context key that carries the destination of a
// DBus message.
type destContextKey struct{}

// ContextDestination returns the destination found in ctx, and
// reports whether a destination was found.
//
// Destination information is available in [Marshaler]'s MarshalDBus
// method when encoding method return values.
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

type allowInteractionContextKey struct{}

// WithContextUserInteraction returns a copy of the parent context
// with the DBus user interation policy set according to allow.
//
// Allowing user interaction prompts the user for permission to
// proceed when a call is made to a privileged method, instead of
// returning an access denied error immediately.
//
// Interaction is disabled by default because it can cause calls to
// block for an extended period of time, until the user responds to
// the authorization prompt, or indefinitely on servers where users
// aren't expected to be available for interactive prompting.
func WithContextUserInteraction(parent context.Context, allow bool) context.Context {
	return context.WithValue(parent, allowInteractionContextKey{}, allow)
}

type blockAutostartContextKey struct{}

// WithContextAutostart returns a copy of the parent context with the
// DBus auto-start policy set according to allow.
//
// Services that provide DBus APIs can elect to be "bus
// activated". Bus activated peers are present on the bus even when
// their underlying service isn't running, and the bus arranges to
// start them seamlessly when something communicates with them.
//
// By default, method calls trigger bus activation, and clients don't
// need to be aware of this feature. If auto-starting services is
// undesirable, WithContextAutostart can be used to make calls fail
// with a [CallError] if they would trigger a bus activation.
func WithContextAutostart(ctx context.Context, allow bool) context.Context {
	return context.WithValue(ctx, blockAutostartContextKey{}, !allow)
}

func contextCallFlags(ctx context.Context) (flags byte) {
	if v, ok := ctx.Value(allowInteractionContextKey{}).(bool); ok && v {
		flags |= 0x4
	}
	if v, ok := ctx.Value(blockAutostartContextKey{}).(bool); ok && v {
		flags |= 0x2
	}
	return flags
}
