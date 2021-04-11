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

// Logger is cronmon's event logger. It writes line-delimited JSON events.
type Logger struct {
	w io.Writer
}

// NewLogger create is a new event writer.
func NewLogger(w io.Writer) *Logger {
	return &Logger{w}
}

// Write writes the given event into the writer. Writes are concurrently safe
// and are atomic.
func (l *Logger) Write(ev Event) error {
	evJSON := EventJSON{
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
