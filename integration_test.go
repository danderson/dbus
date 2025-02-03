package dbus_test

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/danderson/dbus"
	"github.com/danderson/dbus/dbustest"
)

// debugging tests, and the bus monitor output is too much? Turn it
// off temporarily here.
//
// Note that to help pick through the test logs, all dbus-monitor
// output is logged through the same codepath, such that all
// dbus-monitor log entries have the same distinctive source line.
const logBusTraffic = true

func TestBus(t *testing.T) {
	bus := dbustest.New(t, logBusTraffic)

	conn := bus.MustConn(t)
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
	bus := dbustest.New(t, logBusTraffic)

	conn := bus.MustConn(t)
	defer conn.Close()

	busPeer := conn.Peer("org.freedesktop.DBus")
	if got, want := busPeer.Name(), "org.freedesktop.DBus"; got != want {
		t.Errorf("Peer.Name() is wrong, got %q want %q", got, want)
	}
	if busPeer.IsUniqueName() {
		t.Error("IsUniqueName() true for bus peer, want false")
	}
	if err := busPeer.Ping(context.Background()); err != nil {
		t.Errorf("busPeer.Ping() failed: %v", err)
	}

	creds, err := busPeer.Identity(context.Background())
	if err != nil {
		t.Errorf("busPeer.Identity() failed: %v", err)
	} else if creds.UID == nil {
		t.Error("busPeer.Identity() has nil UID")
	} else if creds.PID == nil {
		t.Error("busPeer.Identity() has nil PID")
	}

	//lint:ignore SA1019 testing deprecated method
	uid, err := busPeer.UID(context.Background())
	if err != nil {
		t.Errorf("busPeer.UID() failed: %v", err)
	} else if uid != *creds.UID {
		t.Errorf("busPeer.Identity().UID = %d, but busPeer.UID() = %d", *creds.UID, uid)
	} else if testing.Verbose() {
		t.Logf("busPeer.UID() = %d", uid)
	}

	//lint:ignore SA1019 testing deprecated method
	pid, err := busPeer.PID(context.Background())
	if err != nil {
		t.Errorf("busPeer.PID() failed: %v", err)
	} else if pid != *creds.PID {
		t.Errorf("busPeer.Identity().PID = %d, but busPeer.PID() = %d", *creds.PID, pid)
	} else if testing.Verbose() {
		t.Logf("busPeer.PID() = %d", pid)
	}

	exists, err := busPeer.Exists(context.Background())
	if err != nil {
		t.Errorf("busPeer.Exists() failed: %v", err)
	} else if !exists {
		t.Error("busPeer.Exists() is false but I'm talking to it!")
	}

	owner, err := busPeer.Owner(context.Background())
	if err != nil {
		t.Errorf("busPeer.Owner() failed: %v", err)
	} else if got, want := owner.Name(), "org.freedesktop.DBus"; got != want {
		t.Errorf("busPeer.Owner() = %q, want %q", got, want)
	} else if testing.Verbose() {
		t.Logf("busPeer.Owner() = %s", owner)
	}
}

func TestObject(t *testing.T) {
	bus := dbustest.New(t, logBusTraffic)

	conn := bus.MustConn(t)
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

func TestInterface(t *testing.T) {
	bus := dbustest.New(t, logBusTraffic)

	conn := bus.MustConn(t)
	defer conn.Close()

	// Call with no args, one retval
	busPeer := conn.Peer("org.freedesktop.DBus").Object("/org/freedesktop/DBus").Interface("org.freedesktop.DBus")
	var id string
	if err := busPeer.Call(context.Background(), "GetId", nil, &id); err != nil {
		t.Fatalf("busPeer.GetId failed: %v", err)
	}
	if len(id) == 0 {
		t.Fatal("busPeer.GetId got empty ID")
	}

	// Call with an arg, no retval
	if err := busPeer.Call(context.Background(), "AddMatch", "type='signal'", nil); err != nil {
		t.Fatalf("busPeer.AddMatch failed: %v", err)
	}

	// Call with arg and retval
	req := struct{ A, B string }{
		"org.freedesktop.DBus",
		"Features",
	}
	var resp any
	err := busPeer.Object().Interface("org.freedesktop.DBus.Properties").Call(context.Background(), "Get", req, &resp)
	if err != nil {
		t.Fatalf("busPeer.GetProperty failed: %v", err)
	}
	feats, ok := resp.([]string)
	if !ok {
		t.Fatalf("unexpected response %v", resp)
	}
	if len(feats) == 0 {
		t.Fatal("bus has no features")
	}

	// one-way call
	if err := busPeer.OneWay(context.Background(), "GetId", nil); err != nil {
		t.Fatalf("busPeer.OneWay failed: %v", err)
	}

	// Get property into type
	var feats2 []string
	if err := busPeer.GetProperty(context.Background(), "Features", &feats2); err != nil {
		t.Fatalf("busPeer.GetProperty(Features) failed: %v", err)
	}
	if !slices.Equal(feats, feats2) {
		t.Fatalf("busPeer.GetProperty output differs from manual call:\n  got: %v\n want: %v", feats2, feats)
	}

	// Get property into any
	var resp2 any
	if err := busPeer.GetProperty(context.Background(), "Features", &resp2); err != nil {
		t.Fatalf("busPeer.GetProperty(Features) failed: %v", err)
	}

	if !reflect.DeepEqual(resp, resp2) {
		t.Fatalf("busPeer.GetProperty output to any differs from manual call:\n  got: %v\n want: %v", resp2, resp)
	}

	// Get all properties
	props, err := busPeer.GetAllProperties(context.Background())
	if err != nil {
		t.Fatalf("busPeer.GetAllProperties failed: %v", err)
	}
	if props["Features"] == nil {
		t.Fatal("busPeer.GetAllProperties did not return Features")
	}
	if props["Interfaces"] == nil {
		t.Fatal("busPeer.GetAllProperties did not return Interfaces")
	}

	// Failed call
	err = busPeer.Call(context.Background(), "FlumpoTron", nil, nil)
	if err == nil {
		t.Fatalf("busPeer.FlumpoTron (non-existent method) succeeded")
	}

	// Failed property get
	var jumkle any
	err = busPeer.GetProperty(context.Background(), "JumkleClint", &jumkle)
	if err == nil {
		t.Fatalf("busPeer.GetProperty of non-existent property succeeded")
	}
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
		bus := dbustest.New(t, logBusTraffic)

		conn := bus.MustConn(t)
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
		bus := dbustest.New(t, logBusTraffic)

		conn1 := bus.MustConn(t)
		defer conn1.Close()

		claim1, err := conn1.Claim("org.test.Bus", dbus.ClaimOptions{})
		if err != nil {
			t.Fatalf("conn1.Claim() failed: %v", err)
		}

		awaitOwner(t, claim1, "1", true)

		conn2 := bus.MustConn(t)
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
		bus := dbustest.New(t, logBusTraffic)

		conn1, conn2, conn3 := bus.MustConn(t), bus.MustConn(t), bus.MustConn(t)
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
		bus := dbustest.New(t, logBusTraffic)

		conn1, conn2, conn3 := bus.MustConn(t), bus.MustConn(t), bus.MustConn(t)
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
