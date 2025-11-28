// Package pubsub provides a simple publish-subscribe mechanism for GraphQL subscriptions.
package pubsub

import (
	"sync"
)

// Topic represents a subscription topic.
type Topic string

const (
	TopicDMXOutput         Topic = "DMX_OUTPUT_CHANGED"
	TopicProjectUpdated    Topic = "PROJECT_UPDATED"
	TopicPreviewSession    Topic = "PREVIEW_SESSION_UPDATED"
	TopicCueListPlayback   Topic = "CUE_LIST_PLAYBACK_UPDATED"
	TopicSystemInfo        Topic = "SYSTEM_INFO_UPDATED"
	TopicWiFiStatus        Topic = "WIFI_STATUS_UPDATED"
)

// Subscriber represents a subscription channel.
type Subscriber struct {
	ID      string
	Topic   Topic
	Filter  string // Optional filter value (e.g., projectId, universe)
	Channel chan interface{}
}

// PubSub manages subscriptions and message distribution.
type PubSub struct {
	mu          sync.RWMutex
	subscribers map[Topic][]*Subscriber
	nextID      int
}

// New creates a new PubSub instance.
func New() *PubSub {
	return &PubSub{
		subscribers: make(map[Topic][]*Subscriber),
	}
}

// Subscribe creates a new subscription for a topic.
func (ps *PubSub) Subscribe(topic Topic, filter string, bufferSize int) *Subscriber {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.nextID++
	sub := &Subscriber{
		ID:      string(rune(ps.nextID)),
		Topic:   topic,
		Filter:  filter,
		Channel: make(chan interface{}, bufferSize),
	}

	ps.subscribers[topic] = append(ps.subscribers[topic], sub)
	return sub
}

// Unsubscribe removes a subscription.
func (ps *PubSub) Unsubscribe(sub *Subscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	subs := ps.subscribers[sub.Topic]
	for i, s := range subs {
		if s.ID == sub.ID {
			// Close the channel
			close(s.Channel)
			// Remove from slice
			ps.subscribers[sub.Topic] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// Publish sends a message to all subscribers of a topic.
// If filter is non-empty, only sends to subscribers with matching filter or empty filter.
func (ps *PubSub) Publish(topic Topic, filter string, message interface{}) {
	ps.mu.RLock()
	subs := ps.subscribers[topic]
	ps.mu.RUnlock()

	for _, sub := range subs {
		// Send if no filter or filters match
		if sub.Filter == "" || filter == "" || sub.Filter == filter {
			select {
			case sub.Channel <- message:
				// Message sent
			default:
				// Channel full, skip (non-blocking)
			}
		}
	}
}

// PublishAll sends a message to all subscribers of a topic regardless of filter.
func (ps *PubSub) PublishAll(topic Topic, message interface{}) {
	ps.mu.RLock()
	subs := ps.subscribers[topic]
	ps.mu.RUnlock()

	for _, sub := range subs {
		select {
		case sub.Channel <- message:
			// Message sent
		default:
			// Channel full, skip (non-blocking)
		}
	}
}

// SubscriberCount returns the number of subscribers for a topic.
func (ps *PubSub) SubscriberCount(topic Topic) int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.subscribers[topic])
}
