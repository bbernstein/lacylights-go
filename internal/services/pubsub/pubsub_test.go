package pubsub

import (
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	ps := New()
	if ps == nil {
		t.Fatal("New() returned nil")
	}
	if ps.subscribers == nil {
		t.Error("subscribers map should be initialized")
	}
}

func TestSubscribe(t *testing.T) {
	ps := New()

	sub := ps.Subscribe(TopicDMXOutput, "", 10)
	if sub == nil {
		t.Fatal("Subscribe() returned nil")
	}
	if sub.Topic != TopicDMXOutput {
		t.Errorf("Expected topic %s, got %s", TopicDMXOutput, sub.Topic)
	}
	if sub.Filter != "" {
		t.Errorf("Expected empty filter, got '%s'", sub.Filter)
	}
	if cap(sub.Channel) != 10 {
		t.Errorf("Expected channel buffer size 10, got %d", cap(sub.Channel))
	}

	// Check subscriber count
	if count := ps.SubscriberCount(TopicDMXOutput); count != 1 {
		t.Errorf("Expected 1 subscriber, got %d", count)
	}
}

func TestSubscribe_WithFilter(t *testing.T) {
	ps := New()

	sub := ps.Subscribe(TopicProjectUpdated, "project-123", 5)
	if sub.Filter != "project-123" {
		t.Errorf("Expected filter 'project-123', got '%s'", sub.Filter)
	}
}

func TestSubscribe_MultipleSubscribers(t *testing.T) {
	ps := New()

	ps.Subscribe(TopicDMXOutput, "", 10)
	ps.Subscribe(TopicDMXOutput, "", 10)
	ps.Subscribe(TopicProjectUpdated, "", 10)

	if count := ps.SubscriberCount(TopicDMXOutput); count != 2 {
		t.Errorf("Expected 2 DMX subscribers, got %d", count)
	}
	if count := ps.SubscriberCount(TopicProjectUpdated); count != 1 {
		t.Errorf("Expected 1 Project subscriber, got %d", count)
	}
}

func TestUnsubscribe(t *testing.T) {
	ps := New()

	sub := ps.Subscribe(TopicDMXOutput, "", 10)
	if count := ps.SubscriberCount(TopicDMXOutput); count != 1 {
		t.Errorf("Expected 1 subscriber before unsubscribe, got %d", count)
	}

	ps.Unsubscribe(sub)

	if count := ps.SubscriberCount(TopicDMXOutput); count != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", count)
	}

	// Channel should be closed
	select {
	case _, ok := <-sub.Channel:
		if ok {
			t.Error("Channel should be closed after unsubscribe")
		}
	default:
		t.Error("Channel should be closed and readable")
	}
}

func TestUnsubscribe_NonExistent(t *testing.T) {
	ps := New()

	// Create a fake subscriber that doesn't exist in pubsub
	fakeSub := &Subscriber{
		ID:      "fake-id",
		Topic:   TopicDMXOutput,
		Channel: make(chan interface{}, 1),
	}

	// Should not panic
	ps.Unsubscribe(fakeSub)
}

func TestPublish(t *testing.T) {
	ps := New()

	sub := ps.Subscribe(TopicDMXOutput, "", 10)

	// Publish a message
	ps.Publish(TopicDMXOutput, "", "test message")

	// Should receive the message
	select {
	case msg := <-sub.Channel:
		if msg != "test message" {
			t.Errorf("Expected 'test message', got '%v'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for message")
	}
}

func TestPublish_WithFilter(t *testing.T) {
	ps := New()

	// Subscriber with specific filter
	subWithFilter := ps.Subscribe(TopicProjectUpdated, "project-1", 10)
	// Subscriber with different filter
	subOtherFilter := ps.Subscribe(TopicProjectUpdated, "project-2", 10)
	// Subscriber with no filter (should receive all)
	subNoFilter := ps.Subscribe(TopicProjectUpdated, "", 10)

	// Publish to project-1
	ps.Publish(TopicProjectUpdated, "project-1", "msg for project-1")

	// subWithFilter should receive
	select {
	case msg := <-subWithFilter.Channel:
		if msg != "msg for project-1" {
			t.Errorf("Expected 'msg for project-1', got '%v'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("subWithFilter should have received the message")
	}

	// subOtherFilter should NOT receive
	select {
	case <-subOtherFilter.Channel:
		t.Error("subOtherFilter should not have received the message")
	case <-time.After(50 * time.Millisecond):
		// Expected - no message
	}

	// subNoFilter should receive (empty filter matches all)
	select {
	case msg := <-subNoFilter.Channel:
		if msg != "msg for project-1" {
			t.Errorf("Expected 'msg for project-1', got '%v'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("subNoFilter should have received the message")
	}
}

func TestPublish_EmptyFilter(t *testing.T) {
	ps := New()

	// Subscriber with specific filter
	subWithFilter := ps.Subscribe(TopicProjectUpdated, "project-1", 10)

	// Publish with empty filter (should match all)
	ps.Publish(TopicProjectUpdated, "", "broadcast message")

	// Should receive because filter is empty
	select {
	case msg := <-subWithFilter.Channel:
		if msg != "broadcast message" {
			t.Errorf("Expected 'broadcast message', got '%v'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Should have received message with empty publish filter")
	}
}

func TestPublish_ChannelFull(t *testing.T) {
	ps := New()

	// Create subscriber with buffer size 1
	sub := ps.Subscribe(TopicDMXOutput, "", 1)

	// Fill the channel
	ps.Publish(TopicDMXOutput, "", "msg1")

	// This should not block (non-blocking publish)
	done := make(chan bool, 1)
	go func() {
		ps.Publish(TopicDMXOutput, "", "msg2") // Should be dropped
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(100 * time.Millisecond):
		t.Error("Publish blocked on full channel")
	}

	// Should only have first message
	msg := <-sub.Channel
	if msg != "msg1" {
		t.Errorf("Expected 'msg1', got '%v'", msg)
	}
}

func TestPublishAll(t *testing.T) {
	ps := New()

	sub1 := ps.Subscribe(TopicDMXOutput, "filter1", 10)
	sub2 := ps.Subscribe(TopicDMXOutput, "filter2", 10)
	sub3 := ps.Subscribe(TopicDMXOutput, "", 10)

	// PublishAll should send to all subscribers regardless of filter
	ps.PublishAll(TopicDMXOutput, "broadcast")

	// All should receive
	for i, sub := range []*Subscriber{sub1, sub2, sub3} {
		select {
		case msg := <-sub.Channel:
			if msg != "broadcast" {
				t.Errorf("Subscriber %d: Expected 'broadcast', got '%v'", i, msg)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Subscriber %d timed out waiting for message", i)
		}
	}
}

func TestPublishAll_ChannelFull(t *testing.T) {
	ps := New()

	// Create subscriber with buffer size 1
	sub := ps.Subscribe(TopicDMXOutput, "", 1)

	// Fill the channel
	ps.PublishAll(TopicDMXOutput, "msg1")

	// This should not block
	done := make(chan bool, 1)
	go func() {
		ps.PublishAll(TopicDMXOutput, "msg2")
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("PublishAll blocked on full channel")
	}

	// Drain first message
	<-sub.Channel
}

func TestSubscriberCount(t *testing.T) {
	ps := New()

	// Initially zero
	if count := ps.SubscriberCount(TopicDMXOutput); count != 0 {
		t.Errorf("Expected 0 subscribers initially, got %d", count)
	}

	// Add subscribers
	sub1 := ps.Subscribe(TopicDMXOutput, "", 10)
	sub2 := ps.Subscribe(TopicDMXOutput, "", 10)

	if count := ps.SubscriberCount(TopicDMXOutput); count != 2 {
		t.Errorf("Expected 2 subscribers, got %d", count)
	}

	// Remove one
	ps.Unsubscribe(sub1)
	if count := ps.SubscriberCount(TopicDMXOutput); count != 1 {
		t.Errorf("Expected 1 subscriber after unsubscribe, got %d", count)
	}

	// Remove remaining
	ps.Unsubscribe(sub2)
	if count := ps.SubscriberCount(TopicDMXOutput); count != 0 {
		t.Errorf("Expected 0 subscribers after all unsubscribed, got %d", count)
	}
}

func TestConcurrentOperations(t *testing.T) {
	ps := New()
	var wg sync.WaitGroup

	// Concurrent subscriptions
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := ps.Subscribe(TopicDMXOutput, "", 10)
			// Read a message or timeout
			select {
			case <-sub.Channel:
			case <-time.After(200 * time.Millisecond):
			}
		}()
	}

	// Concurrent publishes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ps.Publish(TopicDMXOutput, "", i)
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()
}

func TestTopicConstants(t *testing.T) {
	// Verify topic constants are distinct
	topics := []Topic{
		TopicDMXOutput,
		TopicProjectUpdated,
		TopicPreviewSession,
		TopicCueListPlayback,
		TopicGlobalPlaybackStatus,
		TopicSystemInfo,
		TopicWiFiStatus,
	}

	seen := make(map[Topic]bool)
	for _, topic := range topics {
		if seen[topic] {
			t.Errorf("Duplicate topic: %s", topic)
		}
		seen[topic] = true
	}
}
