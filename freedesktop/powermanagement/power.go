package powermanagement

import (
	"context"

	"github.com/danderson/dbus"
)

type PowerManagement struct {
	main    dbus.Interface
	inhibit dbus.Interface
}

// New returns an interface to the power management service.
func New(conn *dbus.Conn) PowerManagement {
	obj := conn.Peer("org.freedesktop.PowerManagement").Object("/org/freedesktop/PowerManagement")
	return Interface(obj)
}

// Interface returns a power management interface on the given object.
func Interface(obj dbus.Object) PowerManagement {
	return PowerManagement{
		main:    obj.Interface("org.freedesktop.PowerManagement"),
		inhibit: obj.Interface("org.freedesktop.PowerManagement.Inhibit"),
	}
}

// CanHibernate reports whether the system is capable of hibernating.
//
// Hibernation, also known as "suspend to disk", saves the system
// state to durable storage and powers the computer off entirely.
func (iface PowerManagement) CanHibernate(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.main.Call(ctx, "CanHibernate", nil, &ret)
	return ret, err
}

// CanHybridSuspend reports whether the system is capable of entering
// hybrid sleep.
//
// Hybrid sleep saves the system state to durable storage, but then
// does a regular suspend instead of powering off entirely. This
// allows the system to resume rapidly while it still has battery
// (like suspend), without losing the system state if the battery runs
// out (like hibernate).
func (iface PowerManagement) CanHybridSuspend(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.main.Call(ctx, "CanHybridSuspend", nil, &ret)
	return ret, err
}

// CanSuspend reports whether the system is capable of suspending.
//
// Suspending, also known as "suspend to RAM", puts the system to
// sleep with all its state preserved in RAM.
func (iface PowerManagement) CanSuspend(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.main.Call(ctx, "CanSuspend", nil, &ret)
	return ret, err
}

// CanSuspendThenHibernate reports whether the system is capable of
// "suspend then hibernate" sleep.
//
// Suspend-then-hibernate initially suspends to RAM, but transitions
// to hibernation (suspend to disk) if the battery reaches critical
// levels.
func (iface PowerManagement) CanSuspendThenHibernate(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.main.Call(ctx, "CanSuspendThenHibernate", nil, &ret)
	return ret, err
}

// ShouldSavePower reports whether the caller should try to lower its
// power consumption.
//
// The reported value reports the system's current power usage policy.
// It does not necessarily mean that the system is running on battery
// power.
func (iface PowerManagement) ShouldSavePower(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.main.Call(ctx, "GetPowerSaveStatus", nil, &ret)
	return ret, err
}

// Hibernate asks the system to hibernate.
//
// Hibernation, also known as suspend to disk, saves the running
// system's state to durable storage before powering off entirely. A
// hibernating laptop consumes almost no power, but resuming from
// hibernation takes many seconds.
func (iface PowerManagement) Hibernate(ctx context.Context) error {
	return iface.main.Call(ctx, "Hibernate", nil, nil)
}

// Suspend asks the system to suspend.
//
// Suspending, also known as suspend to RAM, saves the running
// system's state to RAM and goes to sleep. Battery usage while
// suspended is low, but not zero as the system still needs to keep
// the RAM powered on maintain its contents. Resuming from the
// suspended state is very fast, typically under a second.
func (iface PowerManagement) Suspend(ctx context.Context) error {
	return iface.main.Call(ctx, "Suspend", nil, nil)
}

// HasInhibit reports whether the system is currently being prevented
// from sleeping by an application.
//
// Inhibits block all forms of sleep (suspend, hibernate, hybrid
// suspend, suspend-then-hibernate).
func (iface PowerManagement) HasInhibit(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.inhibit.Call(ctx, "HasInhibit", nil, &ret)
	return ret, err
}

// InhibitSleep prevents the system from going to sleep.
//
// application and reason are human-readable strings that should
// explain what is preventing the system from sleeping, and why. For
// example, a background system update might use the application name
// "System" and the reason "Installing updates".
//
// The returned cancellation function should be called when the sleep
// inhibition should be lifted.
func (iface PowerManagement) InhibitSleep(ctx context.Context, application string, reason string) (cancel func(context.Context) error, err error) {
	req := struct{ app, reason string }{application, reason}
	var cookie uint32
	err = iface.inhibit.Call(ctx, "Inhibit", req, &cookie)
	if err != nil {
		return nil, err
	}
	cancel = func(ctx context.Context) error {
		return iface.inhibit.Call(ctx, "UnInhibit", cookie, nil)
	}
	return cancel, nil
}

// CanHibernateChanged signals that the system's ability to hibernate
// has changed.
//
// CanHibernateChanged implements the signal
// org.freedesktop.PowerManagement.CanHibernateChanged.
type CanHibernateChanged struct {
	CanHibernate bool
}

// CanHybridSuspendChanged signals that the system's ability to enter
// hybrid sleep has changed.
//
// CanHybridSuspendChanged implements the signal
// org.freedesktop.PowerManagement.CanHybridSuspendChanged.
type CanHybridSuspendChanged struct {
	CanHybridSuspend bool
}

// CanSuspendChanged signals that the system's ability to suspend to
// RAM has changed.
//
// CanSuspendChanged implements the signal
// org.freedesktop.PowerManagement.CanSuspendChanged.
type CanSuspendChanged struct {
	CanSuspend bool
}

// CanSuspendThenHibernateChanged signals that the system's ability to
// enter "suspend then hibernate" sleep has changed.
//
// CanSuspendThenHibernateChanged implements the signal
// org.freedesktop.PowerManagement.CanSuspendThenHibernateChanged.
type CanSuspendThenHibernateChanged struct {
	CanSuspendThenHibernate bool
}

// ShouldSavePowerChanged signals that the system's power saving
// policy has changed.
//
// ShouldSavePowerChanged implements the signal
// org.freedesktop.PowerManagement.PowerSaveStatusChanged.
type ShouldSavePowerChanged struct {
	SavePower bool
}

// HasInhibitChanged signals that the system's sleep inhibition state
// has changed.
//
// HasInhibitChanged implements the signal
// org.freedesktop.PowerManagement.Inhibit.HasInhibitChanged.
type HasInhibitChanged struct {
	HasInhibit bool
}

func init() {
	dbus.RegisterSignalType[CanHibernateChanged]("org.freedesktop.PowerManagement", "CanHibernateChanged")
	dbus.RegisterSignalType[CanHybridSuspendChanged]("org.freedesktop.PowerManagement", "CanHybridSuspendChanged")
	dbus.RegisterSignalType[CanSuspendChanged]("org.freedesktop.PowerManagement", "CanSuspendChanged")
	dbus.RegisterSignalType[CanSuspendThenHibernateChanged]("org.freedesktop.PowerManagement", "CanSuspendThenHibernateChanged")
	dbus.RegisterSignalType[ShouldSavePowerChanged]("org.freedesktop.PowerManagement", "PowerSaveStatusChanged")
	dbus.RegisterSignalType[HasInhibitChanged]("org.freedesktop.PowerManagement.Inhibit", "HasInhibitChanged")
}
