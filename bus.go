package dbus

import (
	"context"
	"errors"
	"fmt"
)

type NameRequestFlags byte

const (
	NameRequestAllowReplacement NameRequestFlags = 1 << iota
	NameRequestReplace
	NameRequestNoQueue
)

func (c *Conn) RequestName(ctx context.Context, name string, flags NameRequestFlags) (isPrimaryOwner bool, err error) {
	resp, err := Call[uint32](ctx, c.bus, "RequestName", struct {
		Name  string
		Flags uint32
	}{name, uint32(flags)})
	if err != nil {
		return false, err
	}
	switch resp {
	case 1:
		// Became primary owner.
		return true, nil
	case 2:
		// Placed in queue, but not primary.
		return false, nil
	case 3:
		// Couldn't become primary owner, and request flags asked to
		// not queue.
		return false, errors.New("requested name not available")
	case 4:
		// Already the primary owner.
		return true, nil
	default:
		return false, fmt.Errorf("unknown response code %d to RequestName", resp)
	}
}

func (c *Conn) ReleaseName(ctx context.Context, name string) error {
	_, err := Call[uint32](ctx, c.bus, "ReleaseName", name)
	return err
}

func (c *Conn) ListQueuedOwners(ctx context.Context, name string) ([]string, error) {
	return Call[[]string](ctx, c.bus, "ListQueuedOwners", name)
}

func (c *Conn) ListNames(ctx context.Context) ([]string, error) {
	return Call[[]string, any](ctx, c.bus, "ListNames", nil)
}

func (c *Conn) ListActivatableNames(ctx context.Context) ([]string, error) {
	return Call[[]string, any](ctx, c.bus, "ListActivatableNames", nil)
}

func (c *Conn) NameHasOwner(ctx context.Context, name string) (bool, error) {
	return Call[bool](ctx, c.bus, "NameHasOwner", name)
}

func (c *Conn) GetNameOwner(ctx context.Context, name string) (string, error) {
	return Call[string](ctx, c.bus, "GetNameOwner", name)
}

func (c *Conn) GetPeerUID(ctx context.Context, name string) (uint32, error) {
	return Call[uint32](ctx, c.bus, "GetConnectionUnixUser", name)
}

func (c *Conn) GetPeerPID(ctx context.Context, name string) (uint32, error) {
	return Call[uint32](ctx, c.bus, "GetConnectionUnixProcessID", name)
}

type PeerCredentials struct {
	UID           uint32          `dbus:"key=UnixUserID"`
	GIDs          []uint32        `dbus:"key=UnixGroupIDs"`
	PIDFD         *FileDescriptor `dbus:"key=ProcessFD"`
	PID           uint32          `dbus:"key=ProcessID"`
	SID           string          `dbus:"key=WindowsSID"`
	SecurityLabel string          `dbus:"key=LinuxSecurityLabel"`

	Unknown map[string]Variant `dbus:"vardict"`
}

func (c *Conn) GetPeerCredentials(ctx context.Context, name string) (*PeerCredentials, error) {
	return Call[*PeerCredentials](ctx, c.bus, "GetConnectionCredentials", name)
}

func (c *Conn) GetBusID(ctx context.Context) (string, error) {
	return Call[string, any](ctx, c.bus, "GetId", nil)
}

func (c *Conn) Features(ctx context.Context) ([]string, error) {
	return GetProperty[[]string](ctx, c.bus, "Features")
}

// Not implemented:
//  - StartServiceByName, deprecated in favor of auto-start.
//  - UpdateActivationEnvironment, so locked down you can't really do
//    much with it any more, and should really be leaving environment
//    stuff to systemd anyway.
//  - GetAdtAuditSessionData, Solaris-only and so weird even the spec
//    doesn't know wtf it's for.
//  - GetConnectionSELinuxSecurityContext, deprecated in favor
//    of GetConnectionCredentials.
//  - GetMachineID: who cares it's a single computer bus I don't care
//    what the spec thinks
//
// TODO:
//  - AddMatch/RemoveMatch: should be internal only, behind a nicer
//    signals monitoring API.
