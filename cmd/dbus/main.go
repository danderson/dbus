package main

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/creachadair/command"
	"github.com/creachadair/flax"
	"github.com/danderson/dbus"
)

var globalArgs struct {
	UseSessionBus bool `flag:"session,Connect to session bus instead of system bus"`
}

func busConn(ctx context.Context) (*dbus.Conn, error) {
	if globalArgs.UseSessionBus {
		return dbus.SessionBus(ctx)
	} else {
		return dbus.SystemBus(ctx)
	}
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

			command.HelpCommand(nil),
			command.VersionCommand(),
		},
	}

	env := root.NewEnv(nil).MergeFlags(true)
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

	slices.Sort(names)
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
