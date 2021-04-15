package journal

import (
	"encoding/json"
	"io"
	"log"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon"
	"github.com/pkg/errors"
)

// Event describes the JSON structure of an event to be written.
type Event struct {
	Time time.Time     `json:"time"`
	Type string        `json:"type"`
	Data cronmon.Event `json:"data"`
}

// Writer is a simple journaler that writes line-delimited JSON events into the
// writer.
type Writer struct {
	e  *json.Encoder
	id string
}

var _ cronmon.Journaler = (*Writer)(nil)

// NewWriter creates a new journal writer.
func NewWriter(id string, w io.Writer) *Writer {
	return &Writer{json.NewEncoder(w), id}
}

// ID returns the ID of the writer.
func (w *Writer) ID() string { return w.id }

// Write writes the given event into the writer. Writes are concurrently safe
// and are atomic.
func (w *Writer) Write(ev cronmon.Event) error {
	evJSON := Event{
		Time: time.Now(),
		Type: ev.Type(),
		Data: ev,
	}

	// Encode's implementation both does the write in one go and append a new
	// line after each call.
	if err := w.e.Encode(evJSON); err != nil {
		return errors.Wrap(err, "failed to marshal event")
	}

	return nil
}

// HumanWriter writes the journal in a human-friendly format. The format cannot
// be parsed; use a regular Writer for this.
type HumanWriter struct {
	log *log.Logger
	id  string
}

// NewHumanWriter creates a new HumanWriter that writes to the given writer.
func NewHumanWriter(id string, w io.Writer) *HumanWriter {
	logger := log.New(w, "journal: ", log.Ldate|log.Lmicroseconds|log.Lmsgprefix)
	return &HumanWriter{logger, id}
}

// WrapHumanWriter wraps the given logger to return a HumanWriter.
func WrapHumanWriter(id string, logger *log.Logger) *HumanWriter {
	return &HumanWriter{logger, id}
}

func (w *HumanWriter) ID() string { return w.id }

// Write writes the given event into the writer.
func (w *HumanWriter) Write(ev cronmon.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		w.log.Println(ev.Type())
		return nil
	}

	w.log.Printf("%s: %s\n", ev.Type(), string(b))
	return nil
}
