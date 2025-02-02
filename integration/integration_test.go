package dbus_test

import (
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
			mkConn := func() *dbus.Conn {
				ret, err := dbus.Dial(context.Background(), sock)
				if err != nil {
					panic(fmt.Errorf("failed to connect to test bus: %w", err))
				}
				return ret
			}
			stop := func() {
				close(stopCh)
				cmd.Process.Kill()
				<-stoppedCh
			}
			return mkConn, stop
		} else if errors.Is(err, fs.ErrNotExist) {
			time.Sleep(10 * time.Millisecond)
			continue
		} else if err != nil {
			t.Fatalf("waiting for bus socket: %v", err)
		}
	}
}

func TestIntegration(t *testing.T) {
	mkConn, stop := runTestDBus(t)
	defer stop()

	conn := mkConn()
	defer conn.Close()

	if got, want := conn.LocalName(), ":1.0"; got != want {
		t.Errorf("unexpected bus name for conn, got %s want %s", got, want)
	}

	t.Run("Peers", func(t *testing.T) {
		peers, err := conn.Peers(context.Background())
		if err != nil {
			t.Errorf("Peers() failed: %v", err)
		} else {
			wantPeers := []dbus.Peer{
				conn.Peer(":1.0"),
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
	})

	t.Run("ActivatablePeers", func(t *testing.T) {
		peers, err := conn.ActivatablePeers(context.Background())
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
	})

	t.Run("BusID", func(t *testing.T) {
		id, err := conn.BusID(context.Background())
		if err != nil {
			t.Errorf("BusID() failed: %v", err)
		} else if id == "" {
			t.Error("BusID() is empty")
		} else if testing.Verbose() {
			t.Logf("BusID() = %s", id)
		}
	})

	t.Run("Features", func(t *testing.T) {
		features, err := conn.Features(context.Background())
		if err != nil {
			t.Errorf("Features() failed: %v", err)
		} else if !slices.Contains(features, "HeaderFiltering") {
			t.Errorf("Features() is missing HeaderFiltering, got %v", features)
		} else if testing.Verbose() {
			t.Logf("Features() = %v", features)
		}
	})

	t.Run("Peer", func(t *testing.T) {
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
	})

	t.Run("Claim", func(t *testing.T) {
		claim1, err := conn.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Errorf("conn.Claim() failed: %v", err)
		} else if got, want := claim1.Name(), "org.test.Bus"; got != want {
			t.Errorf("claim1.Name() = %q, want %q", got, want)
		} else {
			timeout := time.After(time.Second)
		waitOwner1:
			for {
				select {
				case <-timeout:
					t.Fatalf("failed to get claim1 on bus name")
				case owner := <-claim1.Chan():
					if testing.Verbose() {
						t.Logf("claim1 owner of org.test.Bus: %v", owner)
					}
					if owner {
						break waitOwner1
					}
				}
			}
		}
		defer claim1.Close()

		testPeer := conn.Peer("org.test.Bus")
		owner, err := testPeer.Owner(context.Background())
		if err != nil {
			t.Errorf("testPeer.Owner() failed: %v", err)
		} else if got, want := owner.Name(), ":1.0"; got != want {
			t.Errorf("testPeer.Owner() = %q, want %q", got, want)
		} else if testing.Verbose() {
			t.Logf("testPeer.Owner() = %s", owner)
		}

		conn2 := mkConn()
		defer conn2.Close()

		claim2, err := conn2.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Errorf("conn2.Claim() failed: %v", err)
		}
		select {
		case <-time.After(time.Second):
			t.Fatal("no owner signal for claim2")
		case owner := <-claim2.Chan():
			if owner {
				t.Fatal("claim2 became owner while claim1 still active")
			}
			t.Log(owner)
		}
		select {
		case owner := <-claim2.Chan():
			if owner {
				t.Fatal("claim2 became owner while claim1 still active")
			}
		default:
		}

		owner, err = testPeer.Owner(context.Background())
		if err != nil {
			t.Fatalf("testPeer.Owner() failed: %v", err)
		}
		if got, want := owner.Name(), ":1.0"; got != want {
			t.Fatalf("testPeer has wrong owner %q, want %q", got, want)
		}

		queued, err := testPeer.QueuedOwners(context.Background())
		if err != nil {
			t.Fatalf("testPeer.QueuedOwners() failed: %v", err)
		} else {
			got := fmt.Sprint(queued)
			want := "[:1.0 :1.1]"
			if got != want {
				t.Errorf("testPeer.QueuedOwners() is wrong\n  got: %s\n want: %s", got, want)
			} else if testing.Verbose() {
				t.Logf("testPeer.QueuedOwners() = %s", got)
			}
		}

		claim1.Close()
		timeout := time.After(time.Second)
	waitOwner2:
		for {
			select {
			case <-timeout:
				t.Fatalf("claim1 failed to lose claim on bus name")
			case owner := <-claim1.Chan():
				if testing.Verbose() {
					t.Logf("claim1 owner of org.test.Bus: %v", owner)
				}
				if !owner {
					break waitOwner2
				}
			}
		}
		timeout = time.After(time.Second)
	waitOwner3:
		for {
			select {
			case <-timeout:
				t.Fatalf("claim2 failed to get claim on bus name")
			case owner := <-claim2.Chan():
				if testing.Verbose() {
					t.Logf("claim2 owner of org.test.Bus: %v", owner)
				}
				if owner {
					break waitOwner3
				}
			}
		}
	})

}
