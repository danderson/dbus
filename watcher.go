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

type Signal struct {
	Sender   Interface
	Name     string
	Body     any
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

func (w *Watcher) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	select {
	case <-w.pumpStopped:
		return
	default:
	}

	for m := range w.matches {
		w.conn.removeMatch(context.Background(), m)
	}

	close(w.stopPump)
	close(w.wakePump)
	<-w.pumpStopped
}

func (w *Watcher) Chan() <-chan *Signal {
	return w.signals
}

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
