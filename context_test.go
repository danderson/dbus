package dbus

import (
	"context"
	"os"
	"reflect"
	"slices"
	"testing"
)

func TestContextEmitter(t *testing.T) {
	var conn *Conn
	want := conn.Peer("foo").Object("/bar").Interface("qux")
	ctx := withContextEmitter(context.Background(), want)

	got, ok := ContextEmitter(ctx)
	if !ok {
		t.Fatal("emitter not found in context")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong emitter, got %#v want %#v", got, want)
	}

	got, ok = ContextEmitter(context.Background())
	if ok {
		t.Fatalf("got emitter %#v from context with no emitter", got)
	}
}

func TestContextSender(t *testing.T) {
	var conn *Conn
	want := conn.Peer("foo")
	ctx := withContextSender(context.Background(), want)

	got, ok := ContextSender(ctx)
	if !ok {
		t.Fatal("sender not found in context")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong sender, got %#v want %#v", got, want)
	}

	got, ok = ContextSender(context.Background())
	if ok {
		t.Fatalf("got sender %#v from context with no sender", got)
	}
}

func TestContextDestination(t *testing.T) {
	want := "foo"
	ctx := withContextDestination(context.Background(), want)

	got, ok := ContextDestination(ctx)
	if !ok {
		t.Fatal("destination not found in context")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong destination, got %#v want %#v", got, want)
	}

	got, ok = ContextDestination(context.Background())
	if ok {
		t.Fatalf("got destination %#v from context with no destination", got)
	}
}

func TestContextFile(t *testing.T) {
	var fs []*os.File
	for range 2 {
		f, err := os.CreateTemp(t.TempDir(), "contextfile")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		fs = append(fs, f)
	}
	// ContextFile mutates the passed in file array, keep a separate
	// copy for checking output.
	want := slices.Clone(fs)

	ctx := withContextFiles(context.Background(), fs)

	for i := range 2 {
		got := contextFile(ctx, uint32(i))
		if got == nil {
			t.Fatal("file not found in context")
		}
		if got != want[i] {
			t.Fatalf("wrong file received, got %p, want file %d from %v", got, i, want)
		}
	}

	got := contextFile(ctx, 2)
	if got != nil {
		t.Fatalf("got unexpected file %p after popping all files from %v", got, want)
	}
}
