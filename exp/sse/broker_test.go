package sse

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBrokerFansOutByTopic(t *testing.T) {
	broker := NewBroker(BrokerOptions{})

	first, err := broker.Subscribe("ann", "page:1")
	if err != nil {
		t.Fatal(err)
	}

	second, err := broker.Subscribe("bob", "page:1", "page:2")
	if err != nil {
		t.Fatal(err)
	}

	other, err := broker.Subscribe("cid", "page:3")
	if err != nil {
		t.Fatal(err)
	}

	if delivered := broker.Publish("page:1", Message{Event: "changed"}); delivered != 2 {
		t.Fatalf("delivered to %d subscribers, want 2", delivered)
	}

	for _, subscription := range []*Subscription{first, second} {
		message := <-subscription.Messages()
		if message.Topic != "page:1" || message.Event != "changed" {
			t.Fatalf("unexpected message %+v", message)
		}
	}

	select {
	case message := <-other.Messages():
		t.Fatalf("subscriber of another topic got %+v", message)
	default:
	}
}

func TestBrokerLimits(t *testing.T) {
	broker := NewBroker(BrokerOptions{MaxSubscribers: 3, MaxPerKey: 2})

	first, err := broker.Subscribe("ann", "a")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := broker.Subscribe("ann", "a"); err != nil {
		t.Fatal(err)
	}

	if _, err := broker.Subscribe("ann", "a"); !errors.Is(err, ErrTooManySubscribers) {
		t.Fatalf("third subscription of one key: %v", err)
	}

	if _, err := broker.Subscribe("bob", "a"); err != nil {
		t.Fatal(err)
	}

	if _, err := broker.Subscribe("cid", "a"); !errors.Is(err, ErrTooManySubscribers) {
		t.Fatalf("subscription over the total: %v", err)
	}

	// Closing frees the place, also when closed twice.
	first.Close()
	first.Close()

	if _, err := broker.Subscribe("ann", "a"); err != nil {
		t.Fatalf("a closed subscription kept its place: %v", err)
	}
}

func TestBrokerCoalescesWithSmallBuffer(t *testing.T) {
	broker := NewBroker(BrokerOptions{Buffer: 1})

	subscription, err := broker.Subscribe("", "page")
	if err != nil {
		t.Fatal(err)
	}

	for range 5 {
		broker.Publish("page", Message{Event: "changed"})
	}

	if len(subscription.Messages()) != 1 {
		t.Fatalf("queued %d, want 1", len(subscription.Messages()))
	}
}

func TestBrokerClose(t *testing.T) {
	broker := NewBroker(BrokerOptions{})

	subscription, err := broker.Subscribe("ann", "page")
	if err != nil {
		t.Fatal(err)
	}

	broker.Close()

	if _, ok := <-subscription.Messages(); ok {
		t.Fatal("the subscription stayed open")
	}

	subscription.Close()

	if _, err := broker.Subscribe("ann", "page"); !errors.Is(err, ErrBrokerClosed) {
		t.Fatalf("subscribe after close: %v", err)
	}
}

func TestBrokerConcurrentUse(t *testing.T) {
	broker := NewBroker(BrokerOptions{Buffer: 1})

	var group sync.WaitGroup

	for i := range 8 {
		group.Go(func() {
			for range 100 {
				subscription, err := broker.Subscribe("", "page")
				if err != nil {
					t.Error(err)
					return
				}

				broker.Publish("page", Message{Data: i})
				subscription.Close()
			}
		})
	}

	group.Wait()

	broker.mu.Lock()
	defer broker.mu.Unlock()

	if len(broker.topics) != 0 || broker.count != 0 {
		t.Fatal("subscriptions left behind")
	}
}

func TestServeStreamsMessagesAndHeartbeats(t *testing.T) {
	broker := NewBroker(BrokerOptions{})
	ready := make(chan struct{})
	beats := make(chan struct{}, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subscription, err := broker.Subscribe("", "page")
		if err != nil {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		defer subscription.Close()

		writer := NewWriter(w)
		writer.Flush()
		close(ready)

		_ = writer.Serve(r.Context(), subscription, ServeOptions{
			Heartbeat: 20 * time.Millisecond,
			BeforeHeartbeat: func() error {
				beats <- struct{}{}
				return nil
			},
			Handle: func(w *Writer, message Message) error {
				return w.Signal("changed", message.Data)
			},
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = response.Body.Close() }()

	<-ready
	broker.Publish("page", Message{Data: 7})

	reader := bufio.NewReader(response.Body)
	var seen []string

	for len(seen) < 2 || !slices.ContainsFunc(seen, func(line string) bool { return strings.HasPrefix(line, ": heartbeat") }) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended: %v (seen %q)", err, seen)
		}

		if strings.TrimSpace(line) != "" {
			seen = append(seen, strings.TrimSpace(line))
		}
	}

	if !slices.Contains(seen, `data: {"name":"changed","value":7}`) {
		t.Fatalf("the handled message is missing: %q", seen)
	}

	<-beats
}

func TestServeEndsWhenSubscriptionCloses(t *testing.T) {
	broker := NewBroker(BrokerOptions{})

	subscription, err := broker.Subscribe("", "page")
	if err != nil {
		t.Fatal(err)
	}

	broker.Publish("page", Message{ID: "1", Event: "changed", Data: "hello"})
	subscription.Close()

	recorder := httptest.NewRecorder()
	if err := NewWriter(recorder).Serve(context.Background(), subscription, ServeOptions{}); err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}

	// What the subscription still held is written before Serve ends.
	if body := recorder.Body.String(); body != "id: 1\nevent: changed\ndata: hello\n\n" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestServeWritesMessagesWithoutHandler(t *testing.T) {
	broker := NewBroker(BrokerOptions{})

	subscription, err := broker.Subscribe("", "page")
	if err != nil {
		t.Fatal(err)
	}

	broker.Publish("page", Message{ID: "1", Event: "changed", Data: "hello"})

	ctx, cancel := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()
	done := make(chan error, 1)

	go func() {
		done <- NewWriter(recorder).Serve(ctx, subscription, ServeOptions{})
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve() = %v, want context.Canceled", err)
	}

	if body := recorder.Body.String(); body != "id: 1\nevent: changed\ndata: hello\n\n" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestServeBeforeHeartbeatEndsStream(t *testing.T) {
	broker := NewBroker(BrokerOptions{})

	subscription, err := broker.Subscribe("", "page")
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()

	ended := errors.New("session ended")
	err = NewWriter(httptest.NewRecorder()).Serve(context.Background(), subscription, ServeOptions{
		Heartbeat: time.Millisecond,
		BeforeHeartbeat: func() error {
			return ended
		},
	})

	if !errors.Is(err, ended) {
		t.Fatalf("Serve() = %v, want %v", err, ended)
	}
}
