package dbus

import (
	"context"
	"io"
	"maps"

	"github.com/creachadair/mds/mapset"
	"github.com/danderson/dbus/fragments"
)

func (c *Conn) registerStandardSignals() {
	c.RegisterSignalTypeFunc("org.freedesktop.DBus", "NameOwnerChanged", sigNameOwnerChanged)
	c.RegisterSignalTypeFunc("org.freedesktop.DBus", "NameLost", sigNameLost)
	c.RegisterSignalTypeFunc("org.freedesktop.DBus", "NameAcquired", sigNameAcquired)
	c.RegisterSignalTypeFunc("org.freedesktop.DBus", "ActivatableServicesChanged", sigActivatableServicesChanged)

	c.RegisterSignalTypeFunc("org.freedesktop.DBus.Properties", "PropertiesChanged", sigPropertiesChanged)

	c.RegisterSignalTypeFunc("org.freedesktop.DBus.ObjectManager", "InterfacesAdded", sigInterfacesAdded)
	c.RegisterSignalTypeFunc("org.freedesktop.DBus.ObjectManager", "InterfacesRemoved", sigInterfacesRemoved)
}

type PropertiesChanged struct {
	Interface   Interface
	Changed     map[string]Variant
	Invalidated mapset.Set[string]
}

func sigPropertiesChanged(ctx context.Context, iface Interface, payload io.Reader) (any, error) {
	var body struct {
		Interface   string
		Changed     map[string]Variant
		Invalidated []string
	}
	if err := Unmarshal(ctx, payload, fragments.NativeEndian, &body); err != nil {
		return nil, err
	}

	ret := PropertiesChanged{
		Interface:   iface.Object().Interface(body.Interface),
		Changed:     body.Changed,
		Invalidated: mapset.New(body.Invalidated...),
	}
	return ret, nil
}

type InterfacesAdded struct {
	Object     Object
	Interfaces []Interface
}

func sigInterfacesAdded(ctx context.Context, iface Interface, r io.Reader) (any, error) {
	var body struct {
		Path        ObjectPath
		IfsAndProps map[string]map[string]Variant
	}
	if err := Unmarshal(ctx, r, fragments.NativeEndian, &body); err != nil {
		return nil, err
	}
	ret := InterfacesAdded{
		// TODO: check path is a child of iface.Object()
		Object:     iface.Peer().Object(body.Path),
		Interfaces: make([]Interface, 0, len(body.IfsAndProps)),
	}
	for k := range maps.Keys(body.IfsAndProps) {
		ret.Interfaces = append(ret.Interfaces, ret.Object.Interface(k))
	}
	return ret, nil
}

type InterfacesRemoved struct {
	Object     Object
	Interfaces []Interface
}

func sigInterfacesRemoved(ctx context.Context, iface Interface, r io.Reader) (any, error) {
	var body struct {
		Path ObjectPath
		Ifs  []string
	}
	if err := Unmarshal(ctx, r, fragments.NativeEndian, &body); err != nil {
		return nil, err
	}
	ret := InterfacesRemoved{
		// TODO: check path is a child of iface.Object()
		Object:     iface.Peer().Object(body.Path),
		Interfaces: make([]Interface, 0, len(body.Ifs)),
	}
	for _, k := range body.Ifs {
		ret.Interfaces = append(ret.Interfaces, ret.Object.Interface(k))
	}
	return ret, nil
}

type NameOwnerChanged struct {
	Name string
	Prev *Peer
	New  *Peer
}

func sigNameOwnerChanged(ctx context.Context, iface Interface, r io.Reader) (any, error) {
	var body struct {
		Name, Prev, New string
	}
	if err := Unmarshal(ctx, r, fragments.NativeEndian, &body); err != nil {
		return nil, err
	}
	ret := NameOwnerChanged{
		Name: body.Name,
	}
	if body.Prev != "" {
		p := iface.Conn().Peer(body.Prev)
		ret.Prev = &p
	}
	if body.New != "" {
		n := iface.Conn().Peer(body.New)
		ret.New = &n
	}
	return ret, nil
}

type NameLost struct {
	Name string
}

func sigNameLost(ctx context.Context, _ Interface, r io.Reader) (any, error) {
	var ret NameLost
	if err := Unmarshal(ctx, r, fragments.NativeEndian, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

type NameAcquired struct {
	Name string
}

func sigNameAcquired(ctx context.Context, _ Interface, r io.Reader) (any, error) {
	var ret NameAcquired
	if err := Unmarshal(ctx, r, fragments.NativeEndian, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

type ActivatableServicesChanged struct{}

func sigActivatableServicesChanged(context.Context, Interface, io.Reader) (any, error) {
	return ActivatableServicesChanged{}, nil
}
