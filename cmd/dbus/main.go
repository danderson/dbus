package main

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/creachadair/command"
	"github.com/creachadair/flax"
	"github.com/danderson/dbus"
	"github.com/kr/pretty"
)

var globalArgs struct {
	UseSessionBus bool   `flag:"session,Connect to session bus instead of system bus"`
	Names         string `flag:"names,Comma-separated list of bus names to claim"`
}

func busConn(ctx context.Context) (*dbus.Conn, error) {
	var mk func(context.Context) (*dbus.Conn, error)
	if globalArgs.UseSessionBus {
		mk = dbus.SessionBus
	} else {
		mk = dbus.SystemBus
	}
	conn, err := mk(ctx)
	if err != nil {
		return nil, err
	}

	if globalArgs.Names == "" {
		return conn, nil
	}

	for _, n := range strings.Split(globalArgs.Names, ",") {
		claim, err := conn.Claim(n, dbus.ClaimOptions{})
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("claiming name %q: %w", n, err)
		}
		go func() {
			for isOwner := range claim.Chan() {
				if isOwner {
					fmt.Printf("acquired name %s\n", n)
				} else {
					fmt.Printf("lost name %s\n", n)
				}
			}
		}()
	}

	return conn, nil
}

func main() {
	root := &command.C{
		Name:     "dbus",
		Usage:    "command args...",
		SetFlags: command.Flags(flax.MustBind, &globalArgs),
		Commands: []*command.C{
			{
				Name:  "list",
				Usage: "list",
				Help:  "List peers connected to the bus",
				Run:   command.Adapt(runList),
			},
			{
				Name:  "ping",
				Usage: "ping peer",
				Help:  "Ping a peer",
				Run:   command.Adapt(runPing),
			},
			{
				Name:  "whois",
				Usage: "whois peer",
				Help:  "Get a peer's identity",
				Run:   command.Adapt(runWhois),
			},
			{
				Name:  "listen",
				Usage: "listen",
				Help:  "Listen to bus signals",
				Run:   command.Adapt(runListen),
			},
			{
				Name:  "features",
				Usage: "features",
				Help:  "List the message bus's feature flags",
				Run:   command.Adapt(runFeatures),
			},
			{
				Name:  "introspect",
				Usage: "introspect peer object-path",
				Help:  "Dump the API description for an object",
				Run:   command.Adapt(runIntrospect),
			},
			{
				Name:  "serve-peer",
				Usage: "serve-peer",
				Help:  "Serve the org.freedesktop.DBus.Peer interface",
				Run:   command.Adapt(runServePeer),
			},

			command.HelpCommand(nil),
			command.VersionCommand(),
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	env := root.NewEnv(nil).SetContext(ctx).MergeFlags(true)
	command.RunOrFail(env, os.Args[1:])
}

func runList(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	names, err := conn.Peers(env.Context())
	if err != nil {
		return fmt.Errorf("listing bus names: %w", err)
	}

	slices.SortFunc(names, func(a, b dbus.Peer) int {
		return cmp.Compare(a.Name(), b.Name())
	})
	for _, n := range names {
		fmt.Println(n)
	}

	return nil
}

func runPing(env *command.Env, peer string) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	if err := conn.Peer(peer).Ping(env.Context()); err != nil {
		return fmt.Errorf("Pinging %s: %w", peer, err)
	}

	return nil
}

func runWhois(env *command.Env, peer string) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	creds, err := conn.Peer(peer).Identity(env.Context())
	if err != nil {
		return fmt.Errorf("Getting credentials of %s: %w", peer, err)
	}

	if creds.PID != nil {
		fmt.Println("PID:", *creds.PID)
	}
	if creds.UID != nil {
		fmt.Println("UID:", *creds.UID)
	}
	fmt.Println("GIDs:", creds.GIDs)
	if creds.PIDFD.File != nil {
		fmt.Println("PIDFD:", creds.PIDFD.File.Fd())
	}
	if creds.SecurityLabel != nil {
		fmt.Println("Security label:", string(creds.SecurityLabel))
	}
	for _, k := range slices.Sorted(maps.Keys(creds.Unknown)) {
		fmt.Println(k, "(?):", creds.Unknown[k])
	}

	return nil
}

func runListen(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	w := conn.Watch()
	w.Match(dbus.NewMatch())
	fmt.Println("Listening for signals...")
	for {
		select {
		case <-env.Context().Done():
			return nil
		case sig := <-w.Chan():
			fmt.Printf("Signal %s.%s from %s on object %s:\n  %# v\n", sig.Sender.Name(), sig.Name, sig.Sender.Peer().Name(), sig.Sender.Object().Path(), pretty.Formatter(sig.Body))
			if sig.Overflow {
				fmt.Println("OVERFLOW, some signals lost")
			}
		}
	}
}

func runFeatures(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	features, err := conn.Features(env.Context())
	if err != nil {
		return fmt.Errorf("listing bus features: %w", err)
	}
	slices.Sort(features)
	for _, f := range features {
		fmt.Println(f)
	}
	return nil
}

func runIntrospect(env *command.Env, peer, path string) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	desc, err := conn.Peer(peer).Object(dbus.ObjectPath(path)).Introspect(env.Context())
	if err != nil {
		return fmt.Errorf("Pinging %s: %w", peer, err)
	}
	slices.SortFunc(desc.Interfaces, func(a, b *dbus.InterfaceDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for _, iface := range desc.Interfaces {
		fmt.Println(iface)
	}

	return nil
}

func runServePeer(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	conn.Handle("org.freedesktop.DBus.Peer", "Ping", func(ctx context.Context, path dbus.ObjectPath) error {
		sender, ok := dbus.ContextSender(ctx)
		if !ok {
			panic("no sender in context?")
		}
		fmt.Printf("Got ping on %s from %s\n", path, sender)
		return nil
	})
	conn.Handle("org.freedesktop.DBus.Peer", "GetMachineId", func(ctx context.Context, path dbus.ObjectPath) (string, error) {
		bs, err := os.ReadFile("/etc/machine-id")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(bs)), nil
	})

	<-env.Context().Done()
	fmt.Println("shutdown")
	return nil
}
