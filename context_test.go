package dbus

import (
	"context"
	"os"
	"testing"
)

func TestContextHeader(t *testing.T) {
	var conn *Conn

	tests := []struct {
		name            string
		hdr             header
		wantEmitter     Interface
		wantSender      Peer
		wantDestination Peer
	}{
		{
			name: "call",
			hdr: header{
				Type:        msgTypeCall,
				Version:     1,
				Serial:      1234,
				Path:        "/foo/bar",
				Interface:   "org.test.Foo",
				Member:      "Bar",
				Destination: "org.test.Peer",
			},
			wantEmitter:     Interface{},
			wantSender:      Peer{},
			wantDestination: conn.Peer("org.test.Peer"),
		},

		{
			name: "return",
			hdr: header{
				Type:        msgTypeReturn,
				Version:     1,
				Serial:      1234,
				Sender:      ":1.234",
				Destination: ":2.345",
				ReplySerial: 2,
			},
			wantEmitter:     Interface{},
			wantSender:      conn.Peer(":1.234"),
			wantDestination: conn.Peer(":2.345"),
		},

		{
			name: "error",
			hdr: header{
				Type:        msgTypeError,
				Version:     1,
				Serial:      1234,
				Sender:      ":1.234",
				Destination: ":2.345",
				ReplySerial: 2,
				ErrName:     "org.Test.Error",
			},
			wantEmitter:     Interface{},
			wantSender:      conn.Peer(":1.234"),
			wantDestination: conn.Peer(":2.345"),
		},

		{
			name: "signal",
			hdr: header{
				Type:      msgTypeSignal,
				Version:   1,
				Serial:    1234,
				Sender:    ":1.234",
				Path:      "/foo/bar",
				Interface: "org.test.Interface",
				Member:    "Signal",
			},
			wantEmitter:     conn.Peer(":1.234").Object("/foo/bar").Interface("org.test.Interface"),
			wantSender:      conn.Peer(":1.234"),
			wantDestination: Peer{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := withContextHeader(context.Background(), conn, &tc.hdr)

			gotEmitter, ok := ContextEmitter(ctx)
			wantOK := tc.wantEmitter.Name() != ""
			t.Logf("ContextEmitter() = %s, %v", gotEmitter, ok)
			if ok {
				if got, want := gotEmitter.String(), tc.wantEmitter.String(); got != want {
					t.Errorf("wrong emitter, got %q want %q", got, want)
				}
			} else if wantOK {
				t.Errorf("emitter not found in call context, want %q", tc.wantEmitter)
			}

			gotSender, ok := ContextSender(ctx)
			wantOK = tc.wantSender.Name() != ""
			t.Logf("ContextSender() = %s, %v", gotSender, ok)
			if ok {
				if got, want := gotSender.String(), tc.wantSender.String(); got != want {
					t.Errorf("wrong sender, got %q want %q", got, want)
				}
			} else if wantOK {
				t.Errorf("sender not found in call context, want %q", tc.wantSender)
			}

			gotDestination, ok := ContextDestination(ctx)
			wantOK = tc.wantDestination.Name() != ""
			t.Logf("ContextDestination() = %s, %v", gotDestination, ok)
			if ok {
				if got, want := gotDestination.String(), tc.wantDestination.String(); got != want {
					t.Errorf("wrong destination, got %q want %q", got, want)
				}
			} else if wantOK {
				t.Errorf("destination not found in call context, want %q", tc.wantDestination)
			}
		})
	}
}

func TestContextFile(t *testing.T) {
	var want []*os.File
	for range 2 {
		f, err := os.CreateTemp(t.TempDir(), "contextfile")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		want = append(want, f)
	}

	ctx := withContextFiles(context.Background(), &want)

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
		t.Fatalf("got unexpected file %p from %v", got, want)
	}

	add, err := os.CreateTemp(t.TempDir(), "contextfile")
	if err != nil {
		t.Fatal(err)
	}
	defer add.Close()

	gotIdx, err := contextPutFile(ctx, add)
	if err != nil {
		t.Fatalf("failed to put file: %v", err)
	}
	if want := uint32(2); gotIdx != want {
		t.Fatalf("unexpected file index after put, got %d want %d", gotIdx, want)
	}

	got = contextFile(ctx, 2)
	if got == nil {
		t.Fatal("newly added file not found in context")
	}
	if got != add {
		t.Fatalf("wrong file received, got %p, want %p", got, add)
	}
}
