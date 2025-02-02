package dbus_test

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/danderson/dbus"
)

//go:embed dbus.config
var dbusConfig string

// debugging tests, and the bus monitor output is too much? Turn it
// off temporarily here.
//
// Note that to help pick through the test logs, all dbus-monitor
// output is logged through the same codepath, such that all
// dbus-monitor log entries have the same distinctive source line.
const suppressBusMonitor = false

func TestBus(t *testing.T) {
	mkConn, stop := runTestDBus(t)
	defer stop()

	conn := mkConn()
	defer conn.Close()

	if got, want := conn.LocalName(), ":1.1"; got != want {
		t.Errorf("unexpected bus name for conn, got %s want %s", got, want)
	}

	peers, err := conn.Peers(context.Background())
	if err != nil {
		t.Errorf("Peers() failed: %v", err)
	} else {
		wantPeers := []dbus.Peer{
			conn.Peer(":1.1"),
			conn.Peer("org.freedesktop.DBus"),
		}
		slices.SortFunc(peers, dbus.Peer.Compare)
		got := fmt.Sprint(peers)
		want := fmt.Sprint(wantPeers)
		if got != want {
			t.Errorf("Peers() wrong result:\n  got: %s\n want: %s", got, want)
		}
		if testing.Verbose() {
			t.Logf("Peers() = %s", got)
		}
	}

	peers, err = conn.ActivatablePeers(context.Background())
	t.Log(peers)
	if err != nil {
		t.Errorf("Peers() failed: %v", err)
	} else {
		//if len(peers) != 2 {
		wantPeers := []dbus.Peer{
			conn.Peer("org.freedesktop.DBus"),
			conn.Peer("org.test.Activated"),
		}
		slices.SortFunc(peers, dbus.Peer.Compare)
		got := fmt.Sprint(peers)
		want := fmt.Sprint(wantPeers)
		if got != want {
			t.Errorf("ActivatablePeers() wrong result:\n  got: %s\n want: %s", got, want)
		}
		if testing.Verbose() {
			t.Logf("ActivatablePeers() = %s", got)
		}
	}

	id, err := conn.BusID(context.Background())
	if err != nil {
		t.Errorf("BusID() failed: %v", err)
	} else if id == "" {
		t.Error("BusID() is empty")
	} else if testing.Verbose() {
		t.Logf("BusID() = %s", id)
	}

	features, err := conn.Features(context.Background())
	if err != nil {
		t.Errorf("Features() failed: %v", err)
	} else if !slices.Contains(features, "HeaderFiltering") {
		t.Errorf("Features() is missing HeaderFiltering, got %v", features)
	} else if testing.Verbose() {
		t.Logf("Features() = %v", features)
	}
}

func TestPeer(t *testing.T) {
	mkConn, stop := runTestDBus(t)
	defer stop()

	conn := mkConn()
	defer conn.Close()

	bus := conn.Peer("org.freedesktop.DBus")
	if got, want := bus.Name(), "org.freedesktop.DBus"; got != want {
		t.Errorf("Peer.Name() is wrong, got %q want %q", got, want)
	}
	if bus.IsUniqueName() {
		t.Error("IsUniqueName() true for bus peer, want false")
	}
	if err := bus.Ping(context.Background()); err != nil {
		t.Errorf("bus.Ping() failed: %v", err)
	}

	creds, err := bus.Identity(context.Background())
	if err != nil {
		t.Errorf("bus.Identity() failed: %v", err)
	} else if creds.UID == nil {
		t.Error("bus.Identity() has nil UID")
	} else if creds.PID == nil {
		t.Error("bus.Identity() has nil PID")
	}

	//lint:ignore SA1019 testing deprecated method
	uid, err := bus.UID(context.Background())
	if err != nil {
		t.Errorf("bus.UID() failed: %v", err)
	} else if uid != *creds.UID {
		t.Errorf("bus.Identity().UID = %d, but bus.UID() = %d", *creds.UID, uid)
	} else if testing.Verbose() {
		t.Logf("bus.UID() = %d", uid)
	}

	//lint:ignore SA1019 testing deprecated method
	pid, err := bus.PID(context.Background())
	if err != nil {
		t.Errorf("bus.PID() failed: %v", err)
	} else if pid != *creds.PID {
		t.Errorf("bus.Identity().PID = %d, but bus.PID() = %d", *creds.PID, pid)
	} else if testing.Verbose() {
		t.Logf("bus.PID() = %d", pid)
	}

	exists, err := bus.Exists(context.Background())
	if err != nil {
		t.Errorf("bus.Exists() failed: %v", err)
	} else if !exists {
		t.Error("bus.Exists() is false but I'm talking to it!")
	}

	owner, err := bus.Owner(context.Background())
	if err != nil {
		t.Errorf("bus.Owner() failed: %v", err)
	} else if got, want := owner.Name(), "org.freedesktop.DBus"; got != want {
		t.Errorf("bus.Owner() = %q, want %q", got, want)
	} else if testing.Verbose() {
		t.Logf("bus.Owner() = %s", owner)
	}
}

func TestObject(t *testing.T) {
	mkConn, stop := runTestDBus(t)
	defer stop()

	conn := mkConn()
	defer conn.Close()

	o := conn.Peer("org.freedesktop.DBus").Object("/org/freedesktop/DBus")
	desc, err := o.Introspect(context.Background())
	if err != nil {
		t.Fatalf("introspecting DBus: %v", err)
	}
	if len(desc.Interfaces) < 1 {
		t.Fatal("no interfaces found on DBus object")
	}
	t.Log(len(desc.Interfaces))
}

func awaitOwner(t *testing.T, claim *dbus.Claim, claimName string, wantOwner bool) {
	t.Helper()
	if claimName != "" {
		claimName = "claim " + claimName
	} else {
		claimName = "claim"
	}
	timeout := time.After(2 * time.Second)
	for {
		select {
		case gotOwner := <-claim.Chan():
			if testing.Verbose() {
				t.Logf("%s ownership of %q: %v, want %v", claimName, claim.Name(), gotOwner, wantOwner)
			}
			if gotOwner == wantOwner {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %s ownership of %q to be %v", claimName, claim.Name(), wantOwner)
		}
	}
}

func checkClaim(t *testing.T, conn *dbus.Conn, busName string, owners ...*dbus.Conn) {
	t.Helper()
	p := conn.Peer(busName)
	owner, err := p.Owner(context.Background())
	if err != nil {
		t.Fatalf("getting owner of %q: %v", busName, err)
	}
	if gotOwner, wantOwner := owner.Name(), owners[0].LocalName(); gotOwner != wantOwner {
		t.Fatalf("owner of %q is %q, want %q", busName, gotOwner, wantOwner)
	}
	if testing.Verbose() {
		t.Logf("owner of %q is %q", busName, owner.Name())
	}

	queued, err := p.QueuedOwners(context.Background())
	if err != nil {
		t.Fatalf("getting queued owners of %q: %v", busName, err)
	}
	var wantQueued, gotQueued []string
	for _, c := range owners {
		wantQueued = append(wantQueued, c.LocalName())
	}
	for _, c := range queued {
		gotQueued = append(gotQueued, c.Name())
	}
	if !slices.Equal(gotQueued, wantQueued) {
		t.Fatalf("wrong owner queue for %q:\n  got: %v\n want: %v", busName, gotQueued, wantQueued)
	}
	if testing.Verbose() {
		t.Logf("owner queue of %q is %v", busName, gotQueued)
	}
}

func TestClaim(t *testing.T) {
	t.Run("trivial", func(t *testing.T) {
		mkConn, stop := runTestDBus(t)
		defer stop()

		conn := mkConn()
		defer conn.Close()

		claim, err := conn.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Fatalf("conn.Claim() failed: %v", err)
		} else if got, want := claim.Name(), "org.test.Bus"; got != want {
			t.Fatalf("claim.Name() = %q, want %q", got, want)
		}

		awaitOwner(t, claim, "", true)
		checkClaim(t, conn, "org.test.Bus", conn)
	})

	t.Run("normal succession", func(t *testing.T) {
		mkConn, stop := runTestDBus(t)
		defer stop()

		conn1 := mkConn()
		defer conn1.Close()

		claim1, err := conn1.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Fatalf("conn1.Claim() failed: %v", err)
		}

		awaitOwner(t, claim1, "1", true)

		conn2 := mkConn()
		defer conn2.Close()

		claim2, err := conn2.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Fatalf("conn2.Claim() failed: %v", err)
		}

		awaitOwner(t, claim2, "2", false)
		checkClaim(t, conn1, "org.test.Bus", conn1, conn2)

		claim1.Close()
		awaitOwner(t, claim1, "1", false)
		awaitOwner(t, claim2, "2", true)
		checkClaim(t, conn1, "org.test.Bus", conn2)

		claim1, err = conn1.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Fatalf("conn1.Claim() failed: %v", err)
		}

		awaitOwner(t, claim1, "1b", false)
		checkClaim(t, conn1, "org.test.Bus", conn2, conn1)
	})

	t.Run("force replace", func(t *testing.T) {
		mkConn, stop := runTestDBus(t)
		defer stop()

		conn1, conn2, conn3 := mkConn(), mkConn(), mkConn()
		defer conn1.Close()
		defer conn2.Close()
		defer conn3.Close()

		claim1, err := conn1.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Fatalf("conn1.Claim() failed: %v", err)
		}
		defer claim1.Close()
		awaitOwner(t, claim1, "1", true)

		// TryReplace doesn't replace if the current owner disallows it
		claim2, err := conn2.Claim("org.test.Bus", dbus.ClaimOptions{
			TryReplace: true,
		})
		if err != nil {
			t.Fatalf("conn2.Claim() failed: %v", err)
		}
		defer claim2.Close()
		awaitOwner(t, claim2, "2", false)
		checkClaim(t, conn1, "org.test.Bus", conn1, conn2)

		// Updating AllowReplacement doesn't affect past replacement
		// attempts
		err = claim1.Request(dbus.ClaimOptions{
			AllowReplacement: true,
		})
		if err != nil {
			t.Fatalf("conn1.Request() failed: %v", err)
		}
		checkClaim(t, conn1, "org.test.Bus", conn1, conn2)

		// New replacement attempt succeeds
		claim3, err := conn3.Claim("org.test.Bus", dbus.ClaimOptions{
			AllowReplacement: true,
			TryReplace:       true,
		})
		if err != nil {
			t.Fatalf("conn3.Claim() failed: %v", err)
		}
		defer claim3.Close()

		awaitOwner(t, claim3, "3", true)
		awaitOwner(t, claim1, "1", false)
		checkClaim(t, conn1, "org.test.Bus", conn3, conn1, conn2)

		// Old replacement attempt can retry and take ownership.
		err = claim2.Request(dbus.ClaimOptions{
			TryReplace: true,
		})
		if err != nil {
			t.Fatalf("claim2.Request() failed: %v", err)
		}

		awaitOwner(t, claim2, "2", true)
		awaitOwner(t, claim3, "3", false)
		checkClaim(t, conn1, "org.test.Bus", conn2, conn3, conn1)

		// departure of current owner still works normally
		claim2.Close()
		awaitOwner(t, claim2, "2", false)
		awaitOwner(t, claim3, "3", true)
		checkClaim(t, conn1, "org.test.Bus", conn3, conn1)

		// claim that previously allowed replacement still allows
		// replacement.
		err = claim1.Request(dbus.ClaimOptions{
			TryReplace: true,
		})
		if err != nil {
			t.Fatalf("claim1.Request() failed: %v", err)
		}
		awaitOwner(t, claim1, "1", true)
		awaitOwner(t, claim3, "3", false)
		checkClaim(t, conn1, "org.test.Bus", conn1, conn3)
	})

	t.Run("no queue", func(t *testing.T) {
		mkConn, stop := runTestDBus(t)
		defer stop()

		conn1, conn2, conn3 := mkConn(), mkConn(), mkConn()
		defer conn1.Close()
		defer conn2.Close()
		defer conn3.Close()

		claim1, err := conn1.Claim("org.test.Bus", dbus.ClaimOptions{
			NoQueue: true,
		})
		if err != nil {
			t.Fatalf("conn1.Claim() failed: %v", err)
		}
		awaitOwner(t, claim1, "1", true)
		checkClaim(t, conn1, "org.test.Bus", conn1)

		// No queue claim doesn't get ownership, doesn't join the
		// queue.
		claim2, err := conn2.Claim("org.test.Bus", dbus.ClaimOptions{
			NoQueue: true,
		})
		if err != nil {
			t.Fatalf("conn2.Claim() failed: %v", err)
		}
		awaitOwner(t, claim2, "2", false)
		checkClaim(t, conn1, "org.test.Bus", conn1)

		// Repeat request does the same.
		err = claim2.Request(dbus.ClaimOptions{
			NoQueue: true,
		})
		if err != nil {
			t.Fatalf("claim2.Request() failed: %v", err)
		}
		checkClaim(t, conn1, "org.test.Bus", conn1)

		// Vanishing other owner doesn't transfer ownership.
		claim1.Close()
		awaitOwner(t, claim1, "1", false)
		exists, err := conn1.Peer("org.test.Bus").Exists(context.Background())
		if err != nil {
			t.Fatalf("conn1.Peer.Exists failed: %v", err)
		}
		if exists {
			t.Fatal("org.test.Bus still exists, want no owner")
		}

		// Explicit request gets ownership again
		err = claim2.Request(dbus.ClaimOptions{
			AllowReplacement: true,
			NoQueue:          true,
		})
		if err != nil {
			t.Fatalf("claim2.Request failed: %v", err)
		}

		awaitOwner(t, claim2, "2", true)
		checkClaim(t, conn1, "org.test.Bus", conn2)

		// no-queue replacement, current owner leaves the queue.
		claim1, err = conn1.Claim("org.test.Bus", dbus.ClaimOptions{
			TryReplace: true,
			NoQueue:    true,
		})
		if err != nil {
			t.Fatalf("conn1.Claim failed: %v", err)
		}
		defer claim1.Close()
		awaitOwner(t, claim1, "1", true)
		awaitOwner(t, claim2, "2", false)
		checkClaim(t, conn1, "org.test.Bus", conn1)

		// replacer going away doesn't restore claim2's ownership
		claim1.Close()

		awaitOwner(t, claim1, "1", false)
		exists, err = conn1.Peer("org.test.Bus").Exists(context.Background())
		if err != nil {
			t.Fatalf("conn1.Peer.Exists failed: %v", err)
		}
		if exists {
			t.Fatal("org.test.Bus still exists, want no owner")
		}
	})
}

func runTestDBus(t *testing.T) (mkConn func() *dbus.Conn, stop func()) {
	tmp := t.TempDir()

	svc, err := filepath.Abs("./services")
	if err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(tmp, "bus.config")
	cfg := strings.Replace(dbusConfig, "__SERVICEDIR__", svc, -1)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}

	sock := filepath.Join(tmp, "bus.sock")
	cmd := exec.Command("dbus-daemon", "--config-file="+cfgPath, "--nofork", "--nopidfile", "--nosyslog", "--address=unix:path="+sock)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopCh := make(chan struct{})
	stoppedCh := make(chan struct{})
	go func() {
		defer close(stoppedCh)
		err := cmd.Wait()
		select {
		case <-stopCh:
		default:
			panic(fmt.Errorf("dbus stopped prematurely: %w", err))
		}
	}()

	for {
		if _, err := os.Stat(sock); err == nil {
			break
		} else if errors.Is(err, fs.ErrNotExist) {
			time.Sleep(10 * time.Millisecond)
			continue
		} else if err != nil {
			t.Fatalf("waiting for bus socket: %v", err)
		}
	}

	stoppedMonCh := make(chan struct{})
	// Only start the monitor once the bus is running.
	mon := exec.Command("dbus-monitor", "--address", "unix:path="+sock)
	lw := newLogWriter(t)
	mon.Stdout = lw
	mon.Stderr = lw
	if err := mon.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(stoppedMonCh)
		err := mon.Wait()
		select {
		case <-stopCh:
		default:
			panic(fmt.Errorf("dbus-monitor stopped prematurely: %w", err))
		}
		lw.Flush()
	}()
	// dbus-monitor starts by emitting entries that reflect its own
	// joining then leaving the bus.
	lw.WaitForFirstLine()

	mkConn = func() *dbus.Conn {
		ret, err := dbus.Dial(context.Background(), sock)
		if err != nil {
			panic(fmt.Errorf("failed to connect to test bus: %w", err))
		}
		return ret
	}
	stop = func() {
		close(stopCh)
		cmd.Process.Kill()
		mon.Process.Kill()
		<-stoppedCh
		<-stoppedMonCh
	}
	return mkConn, stop
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
	if suppressBusMonitor {
		return
	}
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

func (l *logWriter) WaitForFirstLine() {
	timeout := time.After(2 * time.Second)
	select {
	case <-l.output:
		return
	case <-timeout:
		l.t.Fatalf("timed out waiting for dbus-monitor output")
	}
}
