package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/creachadair/command"
	"github.com/creachadair/flax"
	"github.com/creachadair/mds/heapq"
	"github.com/creachadair/mds/slice"
	"github.com/danderson/dbus"
	"github.com/danderson/dbus/freedesktop/background"
	"github.com/danderson/dbus/internal/dbusgen"
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
				Usage: "list args...",
				Commands: []*command.C{
					{
						Name:  "peers",
						Usage: "list peers",
						Help:  "List peers connected to the bus.",
						Run:   command.Adapt(runListPeers),
					},
					{
						Name:  "interfaces",
						Usage: "list interfaces [peer] [object] [interface]",
						Help: `List bus interfaces.

With no arguments, enumerates all discoverable interfaces on named bus
services. Unique bus names (like ":1.234") are skipped because many of
them do not expect to be sent RPCs, and do not respond correctly.

With one argument, enumerate all objects of the given peer and the
interfaces they implement.

With two arguments, enumerate all interfaces on the given peer and
object.

With three arguments, list only the exact peer, object and interface
specified.

In all cases, the full API for every interface is shown.

Unless explicitly asked for, the listing omits the three well-known
interfaces that most objects implement:
  org.freedesktop.DBus.Peer
  org.freedesktop.DBus.Properties
  org.freedesktop.DBus.Introspectable
`,
						Run: runListInterfaces,
					},
					{
						Name:  "props",
						Usage: "list props [peer] [object] [interface] [property]",
						Help:  "List properties.",
						Run:   runListProps,
					},
				},
			},
			{
				Name:  "ping",
				Usage: "ping peer",
				Help:  "Ping a peer.",
				Run:   command.Adapt(runPing),
			},
			{
				Name:  "whois",
				Usage: "whois peer",
				Help:  "Get a peer's identity.",
				Run:   command.Adapt(runWhois),
			},
			{
				Name:  "listen",
				Usage: "listen",
				Help:  "Listen to bus signals.",
				Run:   command.Adapt(runListen),
			},
			{
				Name:  "features",
				Usage: "features",
				Help:  "List the message bus's feature flags.",
				Run:   command.Adapt(runFeatures),
			},
			{
				Name:  "serve-peer",
				Usage: "serve-peer",
				Help: `Serve the org.freedesktop.DBus.Peer interface.

The interface is implemented on all objects.

For best results, combine with --names to register a service name on the bus that other tools can target.`,
				Run: command.Adapt(runServePeer),
			},
			{
				Name: "generate",
				Usage: `generate interface
generate peer interface`,
				Help:     "Generate an interface implementation from introspection data",
				SetFlags: command.Flags(flax.MustBind, &generateArgs),
				Run:      runGenerate,
			},

			{
				Name:  "freedesktop",
				Usage: "freedesktop args...",
				Commands: []*command.C{
					{
						Name:  "background",
						Usage: "background args...",
						Commands: []*command.C{
							{
								Name:  "list",
								Usage: "list",
								Help:  "List flatpak apps that are running in the background",
								Run:   command.Adapt(runFdoBackgroundList),
							},
						},
					},
				},
			},
			command.HelpCommand(nil),
			command.VersionCommand(),
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	env := root.NewEnv(nil).SetContext(ctx) //.MergeFlags(true)
	command.RunOrFail(env, os.Args[1:])
}

func runListPeers(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(env.Context(), time.Minute)
	defer cancel()
	peers, err := conn.Peers(ctx)
	if err != nil {
		return fmt.Errorf("listing bus names: %w", err)
	}
	aliases := map[dbus.Peer][]dbus.Peer{}

	for _, p := range peers {
		if p.IsUniqueName() {
			continue
		}
		owner, err := p.Owner(ctx)
		if err != nil {
			fmt.Printf("Getting owner of %s: %v\n", p, err)
			continue
		}
		aliases[owner] = append(aliases[owner], p)
		aliases[p] = []dbus.Peer{owner}
	}
	for _, alias := range aliases {
		slices.SortFunc(alias, func(a, b dbus.Peer) int {
			return cmp.Compare(a.Name(), b.Name())
		})
	}

	for _, p := range peers {
		alias := aliases[p]
		if len(alias) == 0 {
			fmt.Println(p)
		} else {
			var out strings.Builder
			out.WriteString(p.Name())
			out.WriteString(" (")
			for i, a := range alias {
				if i > 0 {
					out.WriteString(", ")
				}
				out.WriteString(a.Name())
			}
			out.WriteString(")")
			fmt.Println(out.String())
		}
	}

	return nil
}

func runListInterfaces(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	args := growTo(env.Args, 3)
	ctx, cancel := context.WithTimeout(env.Context(), time.Minute)
	defer cancel()

	var out indenter
	var prev dbus.Interface
	for p, err := range listPeers(ctx, conn, args[0]) {
		if err != nil {
			out.v(err)
		}
		var ownerName string
		owner, err := p.Owner(ctx)
		if err != nil {
			ownerName = fmt.Sprintf("getting owner: %v", err)
		} else {
			ownerName = owner.Name()
		}
		for iface, err := range listInterfaces(ctx, p, args[1], args[2]) {
			if err != nil {
				out.v(err)
				continue
			}
			if iface.Peer() != prev.Peer() {
				out.indent(0)
				if prev.Peer() != (dbus.Peer{}) {
					out.s("")
				}
				out.f("%s (%s)", iface.Peer().Name(), ownerName)
				out.indent(1)
				out.v(iface.Object().Path())
				out.indent(2)
			} else if iface.Object() != prev.Object() {
				out.indent(1)
				out.v(iface.Object().Path())
				out.indent(2)
			}

			out.v(iface.Description)
			prev = iface.Interface
		}
	}

	return nil
}

func runListProps(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	args := growTo(env.Args, 4)
	pf, err := regexp.Compile(args[3])
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(env.Context(), 10*time.Second)
	defer cancel()
	var out indenter
	var prev dbus.Interface
	for p, err := range listPeers(ctx, conn, args[0]) {
		if err != nil {
			out.indent(0)
			out.v(err)
			continue
		}
		for iface, err := range listInterfaces(ctx, p, args[1], args[2]) {
			if err != nil {
				out.indent(0)
				out.v(err)
				continue
			}
			if len(iface.Description.Properties) == 0 {
				continue
			}

			props, err := iface.GetAllProperties(ctx)
			if err != nil {
				out.indent(0)
				out.v(fmt.Errorf("listing properties of %s: %w", iface, err))
				continue
			}
			ks := slices.Sorted(maps.Keys(props))
			ks = slices.Collect(slice.Select(ks, pf.MatchString))
			if len(ks) == 0 {
				continue
			}

			if iface.Peer() != prev.Peer() {
				out.indent(0)
				out.v(iface.Peer().Name())
				out.indent(1)
				out.v(iface.Object().Path())
			} else if iface.Object() != prev.Object() {
				out.indent(1)
				out.v(iface.Object().Path())
			}
			prev = iface.Interface

			out.indent(2)
			out.v(iface.Name())
			out.indent(3)
			for _, k := range ks {
				if pf.MatchString(k) {
					out.f("%s: %v", k, props[k])
				}
			}
		}
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
		return fmt.Errorf("pinging %s: %w", peer, err)
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
		return fmt.Errorf("getting credentials of %s: %w", peer, err)
	}

	if creds.PID != nil {
		fmt.Println("PID:", *creds.PID)
	}
	if creds.UID != nil {
		fmt.Println("UID:", *creds.UID)
	}
	fmt.Println("GIDs:", creds.GIDs)
	if creds.PIDFD != nil {
		fmt.Println("PIDFD:", creds.PIDFD.Fd())
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
	w.Match(dbus.MatchAllSignals())
	fmt.Println("Listening for signals...")
	for {
		select {
		case <-env.Context().Done():
			return nil
		case sig := <-w.Chan():
			fmt.Printf("Signal %s.%s from %s on object %s:\n  %# v\n\n", sig.Sender.Name(), sig.Name, sig.Sender.Peer().Name(), sig.Sender.Object().Path(), pretty.Formatter(sig.Body))
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

var generateArgs struct {
	PackageName string `flag:"package,default=client,Package name to output"`
	OutFile     string `flag:"out,default=gen.go,Output file path"`
}

func findInterface(ctx context.Context, peer dbus.Peer, wantName string) (*dbus.InterfaceDescription, error) {
	var errs []error
	objs := heapq.New(func(a, b dbus.Object) int {
		return cmp.Compare(a.Path(), b.Path())
	})
	objs.Add(peer.Object("/"))
	for !objs.IsEmpty() {
		obj, _ := objs.Pop()
		introCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		desc, err := obj.Introspect(introCtx)
		if err != nil {
			errs = append(errs, fmt.Errorf("introspecting %s: %w", obj, err))
			continue
		}
		for name, iface := range desc.Interfaces {
			if name == wantName {
				fmt.Printf("Found definition of %s at %s\n", name, obj)
				return iface, nil
			}
		}
		for _, child := range desc.Children {
			objs.Add(obj.Child(child))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return nil, nil
}

func runGenerate(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(env.Context(), time.Minute)
	defer cancel()

	var desc *dbus.InterfaceDescription
	switch len(env.Args) {
	case 0:
		return env.Usagef("generate requires at least one argument.")
	case 1:
		peers, err := conn.Peers(env.Context())
		if err != nil {
			return fmt.Errorf("listing peers: %w", err)
		}
		for _, peer := range peers {
			if peer.IsUniqueName() {
				continue
			}
			desc, err = findInterface(ctx, peer, env.Args[0])
			if err != nil {
				fmt.Println(err)
				continue
			}
			if desc != nil {
				break
			}
		}
		if desc == nil {
			return fmt.Errorf("could not find an object that implements %s on the bus", env.Args[0])
		}
	case 2:
		desc, err = findInterface(ctx, conn.Peer(env.Args[0]), env.Args[1])
		if err != nil {
			return err
		}
		if desc == nil {
			return fmt.Errorf("peer %s does not have an object that implements %s", env.Args[0], env.Args[1])
		}
	}

	f, err := os.Create(generateArgs.OutFile)
	if err != nil {
		return fmt.Errorf("creating output %s: %w", generateArgs.OutFile, err)
	}
	defer f.Close()
	fmt.Fprintf(f, `
package %s

import (
  "context"

  "github.com/danderson/dbus"
)
`, generateArgs.PackageName)
	code, err := dbusgen.Interface(desc)
	if _, err := io.WriteString(f, code); err != nil {
		return fmt.Errorf("writing generated code: %w", err)
	}
	if cloErr := f.Close(); cloErr != nil {
		return fmt.Errorf("closing generated file: %w", err)
	}
	if err != nil {
		return fmt.Errorf("generate interface %s: %w", desc.Name, err)
	}
	fmt.Printf("Wrote generated package to %s\n", generateArgs.OutFile)
	return nil
}

func runFdoBackgroundList(env *command.Env) error {
	conn, err := busConn(env.Context())
	if err != nil {
		return fmt.Errorf("connecting to bus: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(env.Context(), 5*time.Second)
	defer cancel()

	apps, err := background.New(conn).BackgroundApps(ctx)
	if err != nil {
		return fmt.Errorf("listing background apps: %w", err)
	}
	slices.SortFunc(apps, func(a, b background.App) int {
		return cmp.Compare(a.ID, b.ID)
	})
	for _, app := range apps {
		fmt.Println(app.ID, app.Instance, app.Status)
	}
	return nil
}
