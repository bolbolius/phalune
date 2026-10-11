package ipc

import (
	"sort"
	"strings"
	"sync"
)

// Event is a single pub-sub message fanned out to subscribers.
// Topic names are lowercase, dot-separated (e.g. "workspaces", "volume").
type Event struct {
	Topic string
	Data  any
}

// eventEnvelope is the wire form sent to subscribed clients, using the
// "event" key so consumers can identify the source line.
type eventEnvelope struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

const (
	topicWildcard = ""
	eventBuffer   = 16
)

// EventBus fans out events without blocking the sender. The latest
// event per topic is retained for new subscribers.
type EventBus struct {
	mu     sync.RWMutex
	subs   map[string]map[int]chan Event
	last   map[string]Event
	nextID int
}

func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[string]map[int]chan Event), last: make(map[string]Event)}
}

// Subscribe registers a subscriber for one or more topics. An empty list
// subscribes to all topics (wildcard). Returns the event channel and an
// unsubscribe function.
func (b *EventBus) Subscribe(topics []string) (<-chan Event, func()) {
	normalized := normalizeTopics(topics)
	if len(normalized) == 0 {
		normalized = []string{topicWildcard}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan Event, eventBuffer)

	for _, t := range normalized {
		set, ok := b.subs[t]
		if !ok {
			set = make(map[int]chan Event)
			b.subs[t] = set
		}
		set[id] = ch
	}

	// Replay latest state so the subscriber starts warm.
	var replay []Event
	if len(normalized) == 1 && normalized[0] == topicWildcard {
		for _, ev := range b.last {
			replay = append(replay, ev)
		}
	} else {
		for _, t := range normalized {
			if ev, ok := b.last[t]; ok {
				replay = append(replay, ev)
			}
		}
	}
	sort.Slice(replay, func(i, j int) bool { return replay[i].Topic < replay[j].Topic })
	for _, ev := range replay {
		select {
		case ch <- ev:
		default:
		}
	}

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for topic, set := range b.subs {
			if _, ok := set[id]; ok {
				delete(set, id)
				if len(set) == 0 {
					delete(b.subs, topic)
				}
			}
		}
	}
	return ch, unsubscribe
}

// Publish delivers data to every subscriber of topic plus all wildcard
// subscribers.
func (b *EventBus) Publish(topic string, data any) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	if topic == "" {
		return
	}

	b.mu.RLock()
	seen := make(map[chan Event]bool)
	var targets []chan Event
	for pattern, set := range b.subs {
		if MatchTopic(topic, pattern) {
			for _, ch := range set {
				if !seen[ch] {
					seen[ch] = true
					targets = append(targets, ch)
				}
			}
		}
	}
	b.mu.RUnlock()

	ev := Event{Topic: topic, Data: data}
	b.mu.Lock()
	b.last[topic] = ev
	b.mu.Unlock()
	for _, ch := range targets {
		select {
		case ch <- ev:
		default:
		}
	}
}

// MatchTopic reports whether a dotted topic name matches a filter pattern.
// Empty or "*" matches everything; a "foo.*" filter matches any topic
// under the "foo" namespace.
func MatchTopic(topic, filter string) bool {
	if filter == "" || filter == "*" {
		return true
	}
	if strings.HasSuffix(filter, ".*") {
		return strings.HasPrefix(topic, strings.TrimSuffix(filter, ".*")+".")
	}
	return topic == filter
}

func normalizeTopics(topics []string) []string {
	out := make([]string, 0, len(topics))
	seen := make(map[string]bool, len(topics))
	for _, t := range topics {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
