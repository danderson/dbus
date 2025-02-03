package dbusgen_test

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danderson/dbus/dbustest"
	"github.com/danderson/dbus/internal/dbusgen"
	"github.com/google/go-cmp/cmp"
)

//go:embed testdata
var golden embed.FS

func TestGen(t *testing.T) {
	bus := dbustest.New(t, false)
	conn := bus.MustConn(t)

	desc, err := conn.Peer("org.freedesktop.DBus").Object("/org/freedesktop/DBus").Introspect(context.Background())
	if err != nil {
		t.Fatalf("introspecting DBus: %v", err)
	}

	for _, iface := range desc.Interfaces {
		goldenPath := filepath.Join("testdata", iface.Name)
		wantBs, err := golden.ReadFile(goldenPath)
		if err != nil {
			t.Errorf("reading golden file %q: %v", goldenPath, err)
			// Deliberately continue with an empty golden, so the
			// expected output still gets written.
		}
		want := string(wantBs)
		got, err := dbusgen.Interface(iface)
		if err != nil {
			t.Errorf("generating interface %q: %v", iface.Name, err)
			continue
		}
		if diff := cmp.Diff(strings.Split(got, "\n"), strings.Split(want, "\n")); diff != "" {
			gotPath := goldenPath + ".got"
			os.WriteFile(gotPath, []byte(got), 0600)
			t.Errorf("wrong dbusgen output (-got+want, got file written to %s):\n%s", gotPath, diff)
		}
	}
}
