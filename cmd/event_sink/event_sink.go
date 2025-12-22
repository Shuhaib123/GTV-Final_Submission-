package event_sink

import (
	"encoding/json"
	"strconv"
	"sync"
)

// Event represents a single streamed update with an incremental ID.
type Event struct {
	ID      string
	Name    string
	Payload json.RawMessage
}

type subscription struct {
	ch chan Event
}

// EventSubscription represents a subscriber to an EventSink.
type EventSubscription struct {
	Events <-chan Event
	close  func()
}

// Close removes the subscription and stops delivery.
func (es *EventSubscription) Close() {
	if es.close != nil {
		es.close()
		es.close = nil
	}
}

// EventSink stores recent events and fans them out to subscribers.
type EventSink struct {
	mu        sync.Mutex
	buffer    []Event
	maxBuffer int
	nextID    int64
	subs      map[*subscription]struct{}
	closed    bool
}

// NewEventSink creates an EventSink that keeps up to maxBuffer pending events for late joiners.
func NewEventSink(maxBuffer int) *EventSink {
	if maxBuffer <= 0 {
		maxBuffer = 1024
	}
	return &EventSink{
		buffer:    make([]Event, 0, maxBuffer),
		maxBuffer: maxBuffer,
		subs:      make(map[*subscription]struct{}),
	}
}

// Add publishes a new event to the sink.
func (s *EventSink) Add(name string, payload interface{}) error {
	var data []byte
	switch v := payload.(type) {
	case json.RawMessage:
		data = v
	default:
		var err error
		data, err = json.Marshal(v)
		if err != nil {
			return err
		}
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	id := strconv.FormatInt(s.nextID, 10)
	s.nextID++
	ev := Event{ID: id, Name: name, Payload: json.RawMessage(data)}
	s.buffer = append(s.buffer, ev)
	if len(s.buffer) > s.maxBuffer {
		s.buffer = s.buffer[1:]
	}
	subs := make([]*subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.ch <- ev:
		default:
			// drop if subscriber is not keeping up
		}
	}
	return nil
}

// Subscribe creates a subscription and replays events after lastID if provided.
func (s *EventSink) Subscribe(lastID string) *EventSubscription {
	sub := &subscription{ch: make(chan Event, 256)}
	history := s.eventsSince(lastID)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		close(sub.ch)
		return &EventSubscription{Events: sub.ch, close: func() {}}
	}
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	for _, ev := range history {
		sub.ch <- ev
	}
	return &EventSubscription{
		Events: sub.ch,
		close: func() {
			s.removeSubscriber(sub)
		},
	}
}

func (s *EventSink) eventsSince(lastID string) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buffer) == 0 {
		return nil
	}
	start := 0
	if lastID != "" {
		if target, err := strconv.ParseInt(lastID, 10, 64); err == nil {
			for i, ev := range s.buffer {
				if val, err := strconv.ParseInt(ev.ID, 10, 64); err == nil && val > target {
					start = i
					break
				}
			}
		}
	}
	events := make([]Event, len(s.buffer)-start)
	copy(events, s.buffer[start:])
	return events
}

func (s *EventSink) removeSubscriber(sub *subscription) {
	s.mu.Lock()
	if _, ok := s.subs[sub]; ok {
		delete(s.subs, sub)
		close(sub.ch)
	}
	s.mu.Unlock()
}

// Close shuts down the sink and all subscriptions.
func (s *EventSink) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	subs := make([]*subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
		delete(s.subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		close(sub.ch)
	}
}
