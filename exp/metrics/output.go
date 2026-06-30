package metrics

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type (
	// WriterSink writes render metric records as JSON lines to an io.Writer.
	WriterSink struct {
		mu     sync.Mutex
		writer io.Writer
		err    error
	}

	jsonRecord struct {
		Kind            string            `json:"kind,omitempty"`
		Name            string            `json:"name,omitempty"`
		RequestID       string            `json:"requestID,omitempty"`
		TraceID         string            `json:"traceID,omitempty"`
		ParentRequestID string            `json:"parentRequestID,omitempty"`
		PartialID       string            `json:"partialID,omitempty"`
		ParentID        string            `json:"parentID,omitempty"`
		PartialLabel    string            `json:"partialLabel,omitempty"`
		SlotName        string            `json:"slotName,omitempty"`
		Templates       []string          `json:"templates,omitempty"`
		OOB             bool              `json:"oob,omitempty"`
		Method          string            `json:"method,omitempty"`
		Path            string            `json:"path,omitempty"`
		Size            int               `json:"size"`
		Rendered        bool              `json:"rendered"`
		StartedAt       time.Time         `json:"startedAt,omitempty"`
		Duration        string            `json:"duration,omitempty"`
		DurationNS      int64             `json:"durationNS,omitempty"`
		Error           string            `json:"error,omitempty"`
		EventKind       string            `json:"eventKind,omitempty"`
		EventLevel      string            `json:"eventLevel,omitempty"`
		EventMessage    string            `json:"eventMessage,omitempty"`
		EventFields     map[string]any    `json:"eventFields,omitempty"`
		Tags            map[string]string `json:"tags,omitempty"`
	}
)

// NewWriterSink returns a sink that writes one JSON record per line to writer.
func NewWriterSink(writer io.Writer) *WriterSink {
	return &WriterSink{writer: writer}
}

// MarshalJSON emits a transport-friendly representation of a metrics record.
func (record Record) MarshalJSON() ([]byte, error) {
	return json.Marshal(toJSONRecord(record))
}

// Record writes record to the configured writer.
func (sink *WriterSink) Record(record Record) {
	if sink == nil || sink.writer == nil {
		return
	}

	line, err := json.Marshal(record)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.err != nil {
		return
	}
	if err != nil {
		sink.err = err
		return
	}
	_, sink.err = sink.writer.Write(append(line, '\n'))
}

// Err returns the first write or encode error observed by the sink.
func (sink *WriterSink) Err() error {
	if sink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return sink.err
}

func toJSONRecord(record Record) jsonRecord {
	out := jsonRecord{
		Kind:            string(record.Kind),
		Name:            record.Name,
		RequestID:       record.RequestID,
		TraceID:         record.TraceID,
		ParentRequestID: record.ParentRequestID,
		PartialID:       record.PartialID,
		ParentID:        record.ParentID,
		PartialLabel:    record.PartialLabel,
		SlotName:        record.SlotName,
		Templates:       record.Templates,
		OOB:             record.OOB,
		Method:          record.Method,
		Path:            record.Path,
		Size:            record.Size,
		Rendered:        record.Rendered,
		StartedAt:       record.StartedAt,
		Duration:        record.Duration.String(),
		DurationNS:      record.Duration.Nanoseconds(),
		EventKind:       record.EventKind,
		EventLevel:      string(record.EventLevel),
		EventMessage:    record.EventMessage,
		EventFields:     record.EventFields,
		Tags:            record.Tags,
	}
	if record.Error != nil {
		out.Error = record.Error.Error()
	}
	return out
}
