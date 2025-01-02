package dbus

import (
	"context"
	"os"
)

type senderContextKey struct{}

func withContextSender(ctx context.Context, iface Interface) context.Context {
	return context.WithValue(ctx, senderContextKey{}, iface)
}

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

type filesContextKey struct{}

func withContextFiles(ctx context.Context, files []*os.File) context.Context {
	return context.WithValue(ctx, filesContextKey{}, &files)
}

func ContextFile(ctx context.Context) (*os.File, bool) {
	v := ctx.Value(filesContextKey{})
	if v == nil {
		return nil, false
	}
	fsp, ok := v.(*[]*os.File)
	if !ok || fsp == nil {
		return nil, false
	}
	fs := *fsp
	if len(fs) == 0 {
		return nil, false
	}
	ret := fs[0]
	// Zero out the ptr so we don't hang onto the file for the
	// duration of the context, if the caller drops it sooner.
	fs[0], fs = nil, fs[1:]
	*fsp = fs

	return ret, true
}
