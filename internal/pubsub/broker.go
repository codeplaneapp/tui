package pubsub

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/crush/internal/observability"
)

const bufferSize = 64

type Broker[T any] struct {
	subs      map[chan Event[T]]struct{}
	mu        sync.RWMutex
	done      chan struct{}
	shutdown  atomic.Bool
	name      string
	bufSize   int
	subCount  int
	maxEvents int
}

func closedEventChan[T any]() <-chan Event[T] {
	ch := make(chan Event[T])
	close(ch)
	return ch
}

func NewBroker[T any]() *Broker[T] {
	return NewNamedBrokerWithOptions[T]("", bufferSize, 1000)
}

func NewNamedBroker[T any](name string) *Broker[T] {
	return NewNamedBrokerWithOptions[T](name, bufferSize, 1000)
}

func NewBrokerWithOptions[T any](channelBufferSize, maxEvents int) *Broker[T] {
	return NewNamedBrokerWithOptions[T]("", channelBufferSize, maxEvents)
}

func NewNamedBrokerWithOptions[T any](name string, channelBufferSize, maxEvents int) *Broker[T] {
	if channelBufferSize <= 0 {
		channelBufferSize = bufferSize
	}
	return &Broker[T]{
		subs:      make(map[chan Event[T]]struct{}),
		done:      make(chan struct{}),
		name:      name,
		bufSize:   channelBufferSize,
		maxEvents: maxEvents,
	}
}

func (b *Broker[T]) Shutdown() {
	if !b.shutdown.CompareAndSwap(false, true) {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	close(b.done)

	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}

	b.subCount = 0
	observability.SetPubSubSubscribers(b.name, 0)
}

func (b *Broker[T]) Subscribe(ctx context.Context) <-chan Event[T] {
	if b.shutdown.Load() {
		return closedEventChan[T]()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.shutdown.Load() {
		return closedEventChan[T]()
	}

	sub := make(chan Event[T], b.bufSize)
	b.subs[sub] = struct{}{}
	b.subCount++
	observability.SetPubSubSubscribers(b.name, b.subCount)

	go func() {
		<-ctx.Done()

		b.mu.Lock()
		defer b.mu.Unlock()

		if b.shutdown.Load() {
			return
		}

		delete(b.subs, sub)
		close(sub)
		b.subCount--
		observability.SetPubSubSubscribers(b.name, b.subCount)
	}()

	return sub
}

func (b *Broker[T]) GetSubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.subCount
}

func (b *Broker[T]) Publish(t EventType, payload T) {
	if b.shutdown.Load() {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.shutdown.Load() {
		return
	}

	event := Event[T]{Type: t, Payload: payload}
	eventType := string(t)

	if len(b.subs) == 0 {
		observability.RecordPubSubEvent(b.name, eventType, "no_subscribers")
		return
	}
	observability.RecordPubSubEvent(b.name, eventType, "published")

	for sub := range b.subs {
		select {
		case sub <- event:
			observability.RecordPubSubEvent(b.name, eventType, "delivered")
		default:
			// Channel is full, subscriber is slow - skip this event
			// This prevents blocking the publisher
			observability.RecordPubSubEvent(b.name, eventType, "dropped")
		}
	}
}
