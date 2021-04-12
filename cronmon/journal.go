package cronmon

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/pkg/errors"
)

// // TxReader is cronmon's transaction reader. It can read logs backward or
// // forward, and it can seek around.
// type TxReader struct {
// 	r io.ReadSeeker
// 	s *bufio.Scanner
// }

// Journaler describes an event logger.
type Journaler interface {
	Write(Event) error
}

type writerJournaler struct{ w io.Writer }

// NewWriterJournaler creates a new journaler that writes line-delimited JSON
// events into the writer.
func NewWriterJournaler(w io.Writer) Journaler {
	return &writerJournaler{w}
}

// Write writes the given event into the writer. Writes are concurrently safe
// and are atomic.
func (l *writerJournaler) Write(ev Event) error {
	type eventJSON struct {
		Time time.Time `json:"time"`
		Type string    `json:"type"`
		Data Event     `json:"data"`
	}

	evJSON := eventJSON{
		Time: time.Now(),
		Type: ev.Type(),
		Data: ev,
	}

	buf := bytes.Buffer{}
	buf.Grow(512)

	if err := json.NewEncoder(&buf).Encode(evJSON); err != nil {
		return errors.Wrap(err, "failed to marshal event")
	}

	// Append a new line.
	buf.WriteByte('\n')

	_, err := l.w.Write(buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "failed to write event")
	}

	return nil
}
