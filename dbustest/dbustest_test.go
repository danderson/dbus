package dbustest_test

import (
	"context"
	"testing"

	"github.com/danderson/dbus/dbustest"
)

func TestBus(t *testing.T) {
	b := dbustest.New(t, true)
	conn := b.MustConn(t)
	if err := conn.Peer("org.freedesktop.DBus").Ping(context.Background()); err != nil {
		t.Fatalf("failed to ping test bus: %v", err)
	}
}
