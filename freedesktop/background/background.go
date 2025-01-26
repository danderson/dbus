// Package background provides an interface to the Freedesktop Flatpak
// background applications monitor.
//
// This corresponds to the org.freedesktop.background.Monitor service
// on the session bus, which provides a way to find out what Flatpak
// applications are running with no visible GUI.
package background

import (
	"context"

	"github.com/danderson/dbus"
)

type Monitor struct{ iface dbus.Interface }

// New returns an interface to the Flatpak background applications
// monitor.
func New(conn *dbus.Conn) Monitor {
	obj := conn.Peer("org.freedesktop.background.Monitor").Object("/org/freedesktop/background/monitor")
	return Interface(obj)
}

// Interface returns a Monitor on the given object.
func Interface(obj dbus.Object) Monitor {
	return Monitor{
		iface: obj.Interface("org.freedesktop.background.Monitor"),
	}
}

// App is a Flatpak application running in the background.
type App struct {
	_ dbus.InlineLayout

	// ID is the application's Flatpak ID.
	ID string `dbus:"key=app_id"`
	// Instance is the application instance's ID.
	Instance string `dbus:"key=instance"`
	// Status is a status message provided by the application.
	Status string `dbus:"key=message"`

	// Unknown collects any new application attributes that are not
	// yet understood by this package.
	Unknown map[string]any `dbus:"vardict"`
}

// BackgroundApps returns a list of Flatpak applications running in
// the background.
func (iface Monitor) BackgroundApps(ctx context.Context) ([]App, error) {
	var ret []App
	if err := iface.iface.GetProperty(ctx, "BackgroundApps", &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

// BackgroundAppsChanged signals that the list of background apps has
// changed.
type BackgroundAppsChanged []App

func init() {
	dbus.RegisterPropertyChangeType[BackgroundAppsChanged]("org.freedesktop.background.Monitor", "BackgroundApps")
}
