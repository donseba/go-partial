package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	partial "github.com/donseba/go-partial"
)

func TestSinkWithLevelsWritesOnlyThose(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	sink := Sink(log, WithLevels(partial.EventDebug, partial.EventError))
	sink.Emit(nil, partial.Event{Kind: "a", Level: partial.EventDebug, Message: "debug"})
	sink.Emit(nil, partial.Event{Kind: "b", Level: partial.EventWarn, Message: "warn"})
	sink.Emit(nil, partial.Event{Kind: "c", Level: partial.EventError, Message: "error"})

	got := out.String()
	if !strings.Contains(got, "msg=debug") || strings.Contains(got, "msg=warn") || !strings.Contains(got, "msg=error") {
		t.Fatalf("log output = %s", got)
	}
}

func TestNilSinkUsesTheDefaultLoggerWhenEmitting(t *testing.T) {
	sink := Sink(nil)

	var out bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	defer slog.SetDefault(previous)

	sink.Emit(nil, partial.Event{Kind: "x", Level: partial.EventWarn, Message: "later default"})

	if !strings.Contains(out.String(), "later default") {
		t.Fatalf("log output = %q; want the default logger set after the sink", out.String())
	}
}
