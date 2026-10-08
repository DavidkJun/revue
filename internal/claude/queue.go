package claude

import "sync"

// eventQueue hands events from the stdout reader to the sender without a
// size limit. exec closes claude's stdout waitDelay after the process exits
// and drops whatever is still in the pipe, so the reader must keep up with
// claude however slowly the caller reads; one answer's events are small
// enough to hold in memory.
type eventQueue struct {
	mu    sync.Mutex
	evs   []Event
	done  bool
	err   error
	ready chan struct{} // capacity 1; signalled after every push and on close
}

func newEventQueue() *eventQueue {
	return &eventQueue{ready: make(chan struct{}, 1)}
}

func (q *eventQueue) push(evs ...Event) {
	if len(evs) == 0 {
		return
	}
	q.mu.Lock()
	q.evs = append(q.evs, evs...)
	q.mu.Unlock()
	q.signal()
}

// close ends the queue; err is why reading stopped, nil at EOF.
func (q *eventQueue) close(err error) {
	q.mu.Lock()
	q.done, q.err = true, err
	q.mu.Unlock()
	q.signal()
}

func (q *eventQueue) signal() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// next returns the oldest event, waiting for one. Once the queue is closed
// and empty, ok is false and err is the error passed to close.
func (q *eventQueue) next() (ev Event, ok bool, err error) {
	for {
		q.mu.Lock()
		if len(q.evs) > 0 {
			ev = q.evs[0]
			q.evs[0] = Event{}
			q.evs = q.evs[1:]
			q.mu.Unlock()
			return ev, true, nil
		}
		done, err := q.done, q.err
		q.mu.Unlock()
		if done {
			return Event{}, false, err
		}
		<-q.ready
	}
}
