package dbus

import (
	"context"
	"maps"
	"reflect"
	"sync"

	"github.com/creachadair/mds/mapset"
	"github.com/creachadair/mds/queue"
)

const maxWatcherQueue = 20

// Watch watches the bus for signals from other bus participants.
//
// The caller must use [Watcher.Match] to specify the signals to be
// delivered to the Watcher.
func (c *Conn) Watch() *Watcher {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := newWatcher(c)
	c.watchers.Add(w)
	return w
}

// A Watcher delivers signals received from the bus that match a given
// set of filters.
type Watcher struct {
	conn     *Conn
	signals  chan *Signal
	wakePump chan struct{}

	stopPump    chan struct{}
	pumpStopped chan struct{}

	mu      sync.Mutex
	queue   queue.Queue[*Signal]
	matches mapset.Set[*Match]
}

// Signal is a signal received from a bus peer.
type Signal struct {
	// Sender is the interface that emitted the signal.
	Sender Interface
	// Name is the name of the signal.
	Name string
	// Body is the signal payload. It is a pointer to the struct type
	// that was associated with the signal name using
	// RegisterSignalType(), or a pointer to an anonymous struct for
	// signals with no registered payload type.
	Body any
	// Overflow reports that the watcher discarded some signals that
	// followed this one, due to the caller not processing delivered
	// signals fast enough.
	Overflow bool
}

func newWatcher(c *Conn) *Watcher {
	ret := &Watcher{
		conn:        c,
		signals:     make(chan *Signal),
		wakePump:    make(chan struct{}, 1),
		stopPump:    make(chan struct{}),
		pumpStopped: make(chan struct{}),
		matches:     mapset.New[*Match](),
	}
	go ret.pump()
	return ret
}

// Close shuts down the Watcher.
func (w *Watcher) Close() {
	select {
	case <-w.pumpStopped:
		return
	default:
	}

	close(w.stopPump)
	close(w.wakePump)
	<-w.pumpStopped

	w.mu.Lock()
	defer w.mu.Unlock()
	for m := range w.matches {
		w.conn.removeMatch(context.Background(), m)
	}
	w.queue.Clear()
}

// Chan returns the channel on which signals are delivered.
//
// The caller must drain this channel of new signals promptly, to
// avoid overflowing the Watcher's receive queue and losing Signals of
// interest. Missing signals due to an overflow are indicated by the
// Overflow field of the [Signal] that immediately precedes the
// discarded signal(s).
func (w *Watcher) Chan() <-chan *Signal {
	return w.signals
}

// Match requests delivery of signals that match the specification m.
//
// Matches are additive: a signal is delivered if it matches any of
// the Watcher's match specifications.
//
// If the match is added successfully, the returned remove function
// may optionally be used to remove that one match without affecting
// any others.
func (w *Watcher) Match(m *Match) (remove func(), err error) {
	// Prevent later thread-unsafe mutation
	m = m.clone()

	if err = w.conn.addMatch(context.Background(), m); err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.matches.Add(m)
	return func() {
		w.conn.removeMatch(context.Background(), m)
		w.mu.Lock()
		defer w.mu.Unlock()
		delete(w.matches, m)
	}, nil
}

func (w *Watcher) deliver(sender Interface, hdr *header, body reflect.Value) {
	w.mu.Lock()
	defer w.mu.Unlock()

	select {
	case <-w.pumpStopped:
		// raced with a Close, this watcher is done.
		return
	default:
	}

	want := func() bool {
		for m := range maps.Keys(w.matches) {
			if m.matches(hdr, body) {
				return true
			}
		}
		return false
	}()
	if !want {
		return
	}

	if w.queue.Len() >= maxWatcherQueue {
		last, _ := w.queue.Peek(-1)
		last.Overflow = true
		return
	}

	w.queue.Add(&Signal{
		Sender: sender,
		Name:   hdr.Member,
		Body:   body.Interface(),
	})
	if w.queue.Len() == 1 {
		select {
		case w.wakePump <- struct{}{}:
		default:
		}
	}
}

func (w *Watcher) pump() {
	defer close(w.pumpStopped)
	defer close(w.signals)
	for {
		sig := func() *Signal {
			w.mu.Lock()
			defer w.mu.Unlock()
			ret, _ := w.queue.Pop()
			return ret
		}()
		if sig == nil {
			select {
			case <-w.stopPump:
				return
			case <-w.wakePump:
				continue
			}
		}
		select {
		case w.signals <- sig:
		case <-w.stopPump:
			return
		}
	}
}
