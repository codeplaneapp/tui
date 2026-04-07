package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroker_SubscribeReceivesPublishedEvents(t *testing.T) {
	b := NewBroker[string]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	b.Publish(CreatedEvent, "hello")

	select {
	case evt := <-ch:
		assert.Equal(t, CreatedEvent, evt.Type)
		assert.Equal(t, "hello", evt.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	b := NewBroker[string]()
	defer b.Shutdown()

	const numSubs = 3
	channels := make([]<-chan Event[string], numSubs)

	for i := range numSubs {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		channels[i] = b.Subscribe(ctx)
	}

	require.Equal(t, numSubs, b.GetSubscriberCount())

	b.Publish(UpdatedEvent, "broadcast")

	for i, ch := range channels {
		select {
		case evt := <-ch:
			assert.Equal(t, UpdatedEvent, evt.Type)
			assert.Equal(t, "broadcast", evt.Payload, "subscriber %d payload mismatch", i)
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out waiting for event", i)
		}
	}
}

func TestBroker_SlowSubscriberDropsEvents(t *testing.T) {
	// Use a buffer size of 2 so we can fill it quickly.
	b := NewBrokerWithOptions[string](2, 1000)
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	// Fill the buffer completely.
	b.Publish(CreatedEvent, "msg-1")
	b.Publish(CreatedEvent, "msg-2")

	// This publish should not block; the event is dropped for the slow subscriber.
	done := make(chan struct{})
	go func() {
		b.Publish(CreatedEvent, "msg-3")
		close(done)
	}()

	select {
	case <-done:
		// Publisher did not block — success.
	case <-time.After(time.Second):
		t.Fatal("publisher blocked on slow subscriber")
	}

	// Drain the channel and verify we got the first two messages (msg-3 was dropped).
	evt1 := <-ch
	assert.Equal(t, "msg-1", evt1.Payload)
	evt2 := <-ch
	assert.Equal(t, "msg-2", evt2.Payload)
}

func TestBroker_ContextCancelUnsubscribes(t *testing.T) {
	b := NewBroker[string]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	ch := b.Subscribe(ctx)

	require.Equal(t, 1, b.GetSubscriberCount())

	cancel()

	// The channel should be closed after the context is cancelled.
	// Drain any buffered events first, then wait for close.
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				// Channel closed — verify subscriber count decremented.
				assert.Eventually(t, func() bool {
					return b.GetSubscriberCount() == 0
				}, time.Second, 10*time.Millisecond)
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for channel to close after context cancel")
		}
	}
}

func TestBroker_ShutdownClosesAllChannels(t *testing.T) {
	b := NewBroker[string]()

	const numSubs = 5
	channels := make([]<-chan Event[string], numSubs)

	for i := range numSubs {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		channels[i] = b.Subscribe(ctx)
	}

	require.Equal(t, numSubs, b.GetSubscriberCount())

	b.Shutdown()

	for i, ch := range channels {
		select {
		case _, ok := <-ch:
			assert.False(t, ok, "channel %d should be closed after shutdown", i)
		case <-time.After(time.Second):
			t.Fatalf("channel %d was not closed after shutdown", i)
		}
	}

	assert.Equal(t, 0, b.GetSubscriberCount())
}

func TestBroker_ShutdownWaitsToCloseDoneUntilLockHeld(t *testing.T) {
	b := NewBroker[string]()

	locked := true
	b.mu.Lock()
	defer func() {
		if locked {
			b.mu.Unlock()
		}
	}()

	shutdownDone := make(chan struct{})
	go func() {
		b.Shutdown()
		close(shutdownDone)
	}()

	assert.Never(t, func() bool {
		select {
		case <-b.done:
			return true
		default:
			return false
		}
	}, 100*time.Millisecond, 10*time.Millisecond)

	b.mu.Unlock()
	locked = false

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for shutdown to complete")
	}

	select {
	case <-b.done:
	default:
		t.Fatal("done channel was not closed after shutdown completed")
	}
}

func TestBroker_SubscribeDuringShutdownReturnsClosedChannel(t *testing.T) {
	b := NewBroker[string]()

	locked := true
	b.mu.Lock()
	defer func() {
		if locked {
			b.mu.Unlock()
		}
	}()

	shutdownDone := make(chan struct{})
	go func() {
		b.Shutdown()
		close(shutdownDone)
	}()

	require.Eventually(t, b.shutdown.Load, time.Second, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subscribeDone := make(chan<- chan Event[string], 1)
	go func() {
		subscribeDone <- b.Subscribe(ctx)
	}()

	select {
	case ch := <-subscribeDone:
		select {
		case _, ok := <-ch:
			assert.False(t, ok, "channel from subscribe during shutdown should be closed")
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for closed channel from subscribe during shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe blocked while shutdown was in progress")
	}

	b.mu.Unlock()
	locked = false

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for shutdown to complete")
	}

	assert.Equal(t, 0, b.GetSubscriberCount())
}

func TestBroker_SubscribeAfterShutdown(t *testing.T) {
	b := NewBroker[string]()
	b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := b.Subscribe(ctx)

	// Channel returned after shutdown should be immediately closed.
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel from post-shutdown subscribe should be closed")
	case <-time.After(time.Second):
		t.Fatal("timed out; channel from post-shutdown subscribe was not closed")
	}
}

func TestBroker_PublishAfterShutdown(t *testing.T) {
	b := NewBroker[string]()
	b.Shutdown()

	// Publish after shutdown should be a no-op — must not panic.
	assert.NotPanics(t, func() {
		b.Publish(CreatedEvent, "should-be-ignored")
	})
}

func TestBroker_ConcurrentPublishSubscribe(t *testing.T) {
	b := NewBroker[string]()
	defer b.Shutdown()

	const numGoroutines = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// 50 goroutines subscribing concurrently.
	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ch := b.Subscribe(ctx)
			// Read a few events to exercise the channel.
			for range 3 {
				select {
				case <-ch:
				case <-time.After(500 * time.Millisecond):
					return
				}
			}
		}(i)
	}

	// 50 goroutines publishing concurrently.
	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			for j := range 10 {
				b.Publish(CreatedEvent, "event")
				_ = j
			}
		}(i)
	}

	// Wait for all goroutines to finish. The race detector will catch any
	// data races during execution.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines completed without data races.
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent goroutines to finish")
	}
}
