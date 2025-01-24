// Package idle provides an interface to the Freedesktop session
// idleness management and locking DBus API.
//
// For historical reasons, the DBus interface for this API is called
// org.freedesktop.ScreenSaver, which is a bit of a misnomer: the API
// is primarily concerned with managing the locking of a session due
// to idleness, although it also provides a method to explicitly lock
// the session immediately as well.
//
// The API also provides a way for applications to temporarily inhibit
// idleness-based session locking, for example so that movie playback
// isn't disrupted.
package idle

import (
	"context"
	"time"

	"github.com/danderson/dbus"
)

type Idle struct{ iface dbus.Interface }

// New returns an interface to the session locking management service.
func New(conn *dbus.Conn) Idle {
	obj := conn.Peer("org.freedesktop.ScreenSaver").Object("/org/freedesktop/ScreenSaver")
	return Interface(obj)
}

// Interface returns a session locking management interface on the
// given object.
func Interface(obj dbus.Object) Idle {
	return Idle{
		iface: obj.Interface("org.freedesktop.ScreenSaver"),
	}
}

// Locked reports whether the session is currently locked.
func (iface Idle) Locked(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.iface.Call(ctx, "GetActive", nil, &ret)
	return ret, err
}

// LockedTime reports the amount of time the session has been locked,
// or 0 if the session is not locked.
func (iface Idle) LockedTime(ctx context.Context) (time.Duration, error) {
	var seconds uint32
	if err := iface.iface.Call(ctx, "GetActiveTime", nil, &seconds); err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}

// IdleTime reports the amount of time the session has been idle.
//
// A session may be idle with or without being locked. Idleness has no
// precise definition, but usually translates to a lack of
// keyboard/mouse inputs.
func (iface Idle) IdleTime(ctx context.Context) (time.Duration, error) {
	var seconds uint32
	if err := iface.iface.Call(ctx, "GetSessionIdleTime", nil, &seconds); err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}

// Inhibit prevents the session from locking due to being idle.
//
// application and reason are human-readable strings that should
// explain what is preventing idle session from locking, and why.
//
// The returned cancellation function should be called when the idle
// lock inhibition should be lifted.
func (iface Idle) Inhibit(ctx context.Context, application string, reason string) (cancel func(context.Context) error, err error) {
	req := struct{ app, reason string }{application, reason}
	var cookie uint32
	err = iface.iface.Call(ctx, "Inhibit", req, &cookie)
	if err != nil {
		return nil, err
	}
	cancel = func(ctx context.Context) error {
		return iface.iface.Call(ctx, "UnInhibit", cookie, nil)
	}
	return cancel, nil
}

// Lock asks the session to lock immediately.
func (iface Idle) Lock(ctx context.Context) error {
	return iface.iface.Call(ctx, "Lock", nil, nil)
}

// SessionStateChanged signals that the session has become
// locked/unlocked.
//
// ScreenSaverStateChanged implements the
// org.freedesktop.ScreenSaver.ActiveChanged signal.
type SessionStateChanged struct {
	Locked bool
}

func init() {
	dbus.RegisterSignalType[SessionStateChanged]("org.freedesktop.ScreenSaver", "ActiveChanged")
}
