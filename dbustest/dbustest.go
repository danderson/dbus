// Package dbustest provides a helper to run an isolated bus
// instance in tests.
package dbustest

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danderson/dbus"
)

//go:embed dbus.config
var dbusConfig string

//go:embed services/*
var dbusServices embed.FS

// Available reports whether the required binaries are available for
// testing against a real DBus server.
func Available() bool {
	_, err := exec.LookPath("dbus-daemon")
	if err != nil {
		return false
	}
	_, err = exec.LookPath("dbus-monitor")
	return err == nil
}

// Bus is an isolated DBus instance for tests.
type Bus struct {
	bus  *exec.Cmd
	mon  *exec.Cmd
	lw   *logWriter
	sock string

	stop       chan struct{}
	busStopped chan struct{}
	monStopped chan struct{}
}

// New launches a DBus instance dedicated to the calling test.
//
// If [Available] is false, New calls t.Skip to skip the calling test.
//
// If logMonitor is true, the returned bus logs all bus messages using
// t.Logf.
func New(t *testing.T, logMonitor bool) *Bus {
	if !Available() {
		t.Skip("dbus-daemon and dbus-monitor not available, cannot run test bus")
	}
	tmp := t.TempDir()
	svc := filepath.Join(tmp, "services")
	if err := os.Mkdir(svc, 0700); err != nil {
		t.Fatalf("creating dbus services dir: %v", err)
	}

	ents, err := dbusServices.ReadDir("services")
	if err != nil {
		t.Fatalf("reading dbus services dir: %v", err)
	}
	for _, ent := range ents {
		bs, err := dbusServices.ReadFile(filepath.Join("services", ent.Name()))
		if err != nil {
			t.Fatalf("reading dbus service file %q: %v", ent.Name(), err)
		}
		err = os.WriteFile(filepath.Join(svc, ent.Name()), bs, 0600)
		if err != nil {
			t.Fatalf("writing dbus service file %q: %v", ent.Name(), err)
		}
	}

	cfgPath := filepath.Join(tmp, "bus.config")
	cfg := strings.Replace(dbusConfig, "__SERVICEDIR__", svc, -1)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}

	ret := &Bus{
		sock:       filepath.Join(tmp, "bus.sock"),
		stop:       make(chan struct{}),
		busStopped: make(chan struct{}),
		monStopped: make(chan struct{}),
	}

	ret.bus = exec.Command("dbus-daemon", "--config-file="+cfgPath, "--nofork", "--nopidfile", "--nosyslog", "--address=unix:path="+ret.sock)
	ret.bus.Stdout = os.Stdout
	ret.bus.Stderr = os.Stderr
	if err := ret.bus.Start(); err != nil {
		t.Fatalf("starting bus: %v", err)
	}
	t.Cleanup(ret.close)

	go func() {
		defer close(ret.busStopped)
		err := ret.bus.Wait()
		select {
		case <-ret.stop:
		default:
			panic(fmt.Errorf("bus stopped prematurely: %w", err))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		if _, err := os.Stat(ret.sock); err == nil {
			break
		} else if errors.Is(err, fs.ErrNotExist) {
			time.Sleep(10 * time.Millisecond)
			continue
		} else if err != nil {
			t.Fatalf("waiting for bus socket: %v", err)
		}
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("bus failed to start: %v", err)
	}

	if logMonitor {
		ret.lw = newLogWriter(t)
		ret.mon = exec.Command("dbus-monitor", "--address", "unix:path="+ret.sock)
		ret.mon.Stdout = ret.lw
		ret.mon.Stderr = ret.lw
		if err := ret.mon.Start(); err != nil {
			t.Fatalf("starting monitor: %v", err)
		}
		go func() {
			defer close(ret.monStopped)
			err := ret.mon.Wait()
			select {
			case <-ret.stop:
			default:
				panic(fmt.Errorf("dbus-monitor stopped prematurely: %w", err))
			}
			ret.lw.Flush()
		}()
		if err := ret.lw.WaitForFirstLine(ctx); err != nil {
			t.Fatalf("waiting for monitor: %v", err)
		}
	} else {
		close(ret.monStopped)
	}

	return ret
}

func (b *Bus) close() {
	close(b.stop)
	b.bus.Process.Kill()
	if b.mon != nil {
		b.mon.Process.Kill()
	}
	timeout := time.After(10 * time.Second)
	select {
	case <-b.busStopped:
	case <-timeout:
		log.Print("timed out waiting for bus to stop")
	}
	select {
	case <-b.monStopped:
	case <-timeout:
		log.Print("timed out waiting for dbus-monitor to stop")
	}
}

// Socket returns the path to the bus's unix socket.
func (b *Bus) Socket() string {
	return b.sock
}

// MustConn returns a connection to the bus. It causes an immediate
// test failure with t.Fatal if it is unable to connect.
func (b *Bus) MustConn(t *testing.T) *dbus.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ret, err := dbus.Dial(ctx, b.sock)
	if err != nil {
		t.Fatalf("connecting to test bus: %v", err)
	}
	return ret
}

type logWriter struct {
	output chan struct{}
	t      *testing.T
	buf    bytes.Buffer
}

func newLogWriter(t *testing.T) *logWriter {
	return &logWriter{
		output: make(chan struct{}, 1),
		t:      t,
	}
}

func (l *logWriter) out(s string) {
	l.t.Log(s)
}

func (l *logWriter) Flush() {
	l.flushComplete()
	l.out(l.buf.String())
	l.buf.Reset()
}

func (l *logWriter) Write(bs []byte) (int, error) {
	l.buf.Write(bs)
	l.flushComplete()
	return len(bs), nil
}

func (l *logWriter) flushComplete() {
	bs := l.buf.Bytes()
	total := 0
	for {
		i := bytes.IndexByte(bs, '\n')
		if i == -1 {
			return
		}
		total += i
		bs = bs[i+1:]
		if !bytes.HasPrefix(bs, []byte("method ")) && !bytes.HasPrefix(bs, []byte("signal ")) && !bytes.HasPrefix(bs, []byte("error ")) {
			total++
			continue
		}

		out := l.buf.Next(total)
		l.out(string(out))
		l.buf.Next(1)
		select {
		case l.output <- struct{}{}:
		default:
		}
		total = 0
		bs = l.buf.Bytes()
	}
}

func (l *logWriter) WaitForFirstLine(ctx context.Context) error {
	select {
	case <-l.output:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
