package sse

import (
	"errors"
	"slices"
	"sync"
)

// Message is one event published to the subscribers of a topic.
type Message struct {
	// Topic is where the message was published.
	Topic string
	// ID, Event and Data are written as the event's fields by Serve when no
	// handler takes the message.
	ID    string
	Event EventName
	Data  any
}

// BrokerOptions limit a Broker.
type BrokerOptions struct {
	// MaxSubscribers caps the open subscriptions; zero is unlimited.
	MaxSubscribers int
	// MaxPerKey caps the open subscriptions that share a key, such as the
	// streams of one user; zero is unlimited.
	MaxPerKey int
	// Buffer is the number of messages a subscription holds while its
	// stream is busy; zero means 16. Messages for a full subscription are
	// dropped, so a slow client never blocks Publish. A buffer of 1 coalesces "something changed" notifications:
	// the subscriber sees one, however many were published meanwhile.
	Buffer int
}

var (
	// ErrTooManySubscribers is returned by Subscribe when a limit of
	// BrokerOptions is reached.
	ErrTooManySubscribers = errors.New("sse: too many subscribers")
	// ErrBrokerClosed is returned by Subscribe after Close.
	ErrBrokerClosed = errors.New("sse: broker closed")
)

// Broker fans messages out to subscriptions by topic, for streams that
// many clients follow, such as everyone looking at the same page. It keeps
// its state in memory: it serves the streams of one process.
//
// A Broker is safe for concurrent use.
type Broker struct {
	options BrokerOptions

	mu     sync.Mutex
	topics map[string]map[*Subscription]struct{}
	keys   map[string]int
	count  int
	closed bool
}

// NewBroker returns a broker with the given limits.
func NewBroker(options BrokerOptions) *Broker {
	if options.Buffer <= 0 {
		options.Buffer = 16
	}

	return &Broker{
		options: options,
		topics:  make(map[string]map[*Subscription]struct{}),
		keys:    make(map[string]int),
	}
}

// Subscribe opens a subscription to topics. key groups subscriptions for
// BrokerOptions.MaxPerKey, for example a user ID; an empty key is not
// limited per key. Close the subscription when the stream ends.
func (b *Broker) Subscribe(key string, topics ...string) (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrBrokerClosed
	}

	if b.options.MaxSubscribers > 0 && b.count >= b.options.MaxSubscribers {
		return nil, ErrTooManySubscribers
	}

	if key != "" && b.options.MaxPerKey > 0 && b.keys[key] >= b.options.MaxPerKey {
		return nil, ErrTooManySubscribers
	}

	subscription := &Subscription{
		broker:   b,
		key:      key,
		messages: make(chan Message, b.options.Buffer),
	}

	b.count++

	if key != "" {
		b.keys[key]++
	}

	b.follow(subscription, topics)

	return subscription, nil
}

// Publish sends message to every subscription of topic and reports how many
// received it. The message's Topic is set to topic. Publish never blocks:
// a subscription whose buffer is full misses the message.
func (b *Broker) Publish(topic string, message Message) int {
	message.Topic = topic

	b.mu.Lock()
	defer b.mu.Unlock()

	delivered := 0

	for subscription := range b.topics[topic] {
		select {
		case subscription.messages <- message:
			delivered++
		default:
			// The subscription's buffer is full: it misses the message.
		}
	}

	return delivered
}

// Close ends every subscription, so their streams finish, for example when
// the server shuts down. Later calls to Subscribe fail with
// ErrBrokerClosed.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true

	for _, subscriptions := range b.topics {
		for subscription := range subscriptions {
			b.end(subscription)
		}
	}
}

// follow subscribes to topics; the caller holds the lock.
func (b *Broker) follow(subscription *Subscription, topics []string) {
	for _, topic := range topics {
		if slices.Contains(subscription.topics, topic) {
			continue
		}

		subscribers := b.topics[topic]
		if subscribers == nil {
			subscribers = make(map[*Subscription]struct{})
			b.topics[topic] = subscribers
		}

		subscribers[subscription] = struct{}{}
		subscription.topics = append(subscription.topics, topic)
	}
}

// unfollow drops the subscription's topics; the caller holds the lock.
func (b *Broker) unfollow(subscription *Subscription) {
	for _, topic := range subscription.topics {
		delete(b.topics[topic], subscription)

		if len(b.topics[topic]) == 0 {
			delete(b.topics, topic)
		}
	}

	subscription.topics = nil
}

// end closes a subscription; the caller holds the lock. Closing the channel
// under the lock keeps Publish from sending on it.
func (b *Broker) end(subscription *Subscription) {
	if subscription.closed {
		return
	}

	b.unfollow(subscription)

	subscription.closed = true
	b.count--

	if subscription.key != "" {
		b.keys[subscription.key]--

		if b.keys[subscription.key] <= 0 {
			delete(b.keys, subscription.key)
		}
	}

	close(subscription.messages)
}

// Subscription receives the messages of its topics.
type Subscription struct {
	broker   *Broker
	key      string
	messages chan Message

	// topics and closed are guarded by the broker's lock.
	topics []string
	closed bool
}

// Messages is the subscription's queue; it is closed when the subscription
// or the broker closes.
func (s *Subscription) Messages() <-chan Message {
	return s.messages
}

// Close ends the subscription and frees its place in the broker's limits.
// It is safe to call more than once.
func (s *Subscription) Close() {
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()

	s.broker.end(s)
}
