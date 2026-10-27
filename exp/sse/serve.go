package sse

import (
	"context"
	"errors"
	"net/http"
	"time"
)

var errWriterNotInitialized = errors.New("sse writer is not initialized")

// ServeOptions configure Writer.Serve.
type ServeOptions struct {
	// Heartbeat is the interval at which Serve writes a comment, which keeps
	// proxies from closing an idle connection and lets the server notice a
	// client that went away. Zero sends none.
	Heartbeat time.Duration
	// BeforeHeartbeat runs before every heartbeat; an error ends Serve with
	// that error, for example when the user's session ended meanwhile.
	BeforeHeartbeat func() error
	// Handle writes a message to the stream, for example a partial rendered
	// for this client. Nil writes the message's ID, Event and Data.
	Handle func(w *Writer, message Message) error
}

// heartbeatComment is the comment written as heartbeat.
const heartbeatComment = "heartbeat"

// Heartbeat writes a comment line, which clients ignore, to keep an idle
// stream open.
func (s *Writer) Heartbeat() error {
	return s.Comment(heartbeatComment)
}

// Serve streams the subscription's messages until ctx ends, the subscription
// closes or a write fails, flushing after every event. It returns nil when
// the subscription closed and the error that ended it otherwise; ctx's error
// when the client went away.
//
// Serve clears the connection's write deadline, as an http.Server's
// WriteTimeout would otherwise end every stream; heartbeats keep detecting
// clients that disappeared.
func (s *Writer) Serve(ctx context.Context, subscription *Subscription, options ServeOptions) error {
	if s == nil || s.w == nil {
		return errWriterNotInitialized
	}

	// Not every writer supports deadlines, such as a test recorder; then
	// there is none to clear.
	_ = http.NewResponseController(s.w).SetWriteDeadline(time.Time{})

	var heartbeat <-chan time.Time

	if options.Heartbeat > 0 {
		ticker := time.NewTicker(options.Heartbeat)
		defer ticker.Stop()

		heartbeat = ticker.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-subscription.Messages():
			if !ok {
				return nil
			}

			if err := s.handle(message, options.Handle); err != nil {
				return err
			}
		case <-heartbeat:
			if err := s.beat(options.BeforeHeartbeat); err != nil {
				return err
			}
		}

		s.Flush()
	}
}

// beat writes a heartbeat of Serve, after before, unless before fails; see
// ServeOptions.BeforeHeartbeat.
func (s *Writer) beat(before func() error) error {
	if before != nil {
		if err := before(); err != nil {
			return err
		}
	}

	return s.Heartbeat()
}

// handle writes a message of Serve with handle, or its ID, Event and Data
// without one; see ServeOptions.Handle.
func (s *Writer) handle(message Message, handle func(w *Writer, message Message) error) error {
	if handle != nil {
		return handle(s, message)
	}

	return s.EventID(message.ID, message.Event, message.Data)
}
