package dbus

import (
	"context"
	"errors"
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
	return context.WithValue(ctx, filesContextKey{}, files)
}

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

type writeFilesContextKey struct{}

func withContextPutFiles(ctx context.Context, files *[]*os.File) context.Context {
	return context.WithValue(ctx, writeFilesContextKey{}, files)
}

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
