package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon"
	"github.com/gofrs/flock"
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
type Writer struct{ w io.Writer }

var _ cronmon.Journaler = (*Writer)(nil)

// NewWriter creates a new journal writer.
func NewWriter(w io.Writer) Writer {
	return Writer{w}
}

// Write writes the given event into the writer. Writes are concurrently safe
// and are atomic.
func (l Writer) Write(ev cronmon.Event) error {
	evJSON := Event{
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

// FileLockJournaler is a journaler that uses a file lock (flock) to lock the
// given file and writes to it. The FileLockJournaler instance must be closed by
// the caller or by the operating system when the application exits.
//
// Reading the Journal
//
// The caller does not need to acquire a file lock in order to read the written
// journal, as each Write operation performed on the file is guaranteed to
// always be valid and atomic.
//
// To read the log, simply use Reader, which is implemented with a line reader
// and a known index to point to the last known length of the file.
type FileLockJournaler struct {
	Writer
	f *os.File
	l *flock.Flock
}

// NewFileLockJournaler creates a new file journaler if it can acquire a flock
// on the path. It returns an error if it fails to acquire the lock.
func NewFileLockJournaler(path string) (*FileLockJournaler, error) {
	return newFileLockJournaler(nil, path)
}

// NewFileLockJournalerWait creates a new file journaler but waits until the
// lock can be acquired or until the context times out.
func NewFileLockJournalerWait(ctx context.Context, path string) (*FileLockJournaler, error) {
	return newFileLockJournaler(ctx, path)
}

func newFileLockJournaler(ctx context.Context, path string) (*FileLockJournaler, error) {
	l := flock.New(path)

	var locked bool
	var err error

	if ctx != nil {
		locked, err = l.TryLockContext(ctx, 25*time.Millisecond)
	} else {
		locked, err = l.TryLock()
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to acquire lock")
	}

	if !locked {
		return nil, errors.New("lock not acquired")
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_SYNC, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file")
	}

	return &FileLockJournaler{
		Writer: Writer{f},
		f:      f,
		l:      l,
	}, nil
}

// Close closes the file and releases the flock.
func (f *FileLockJournaler) Close() error {
	f.f.Close()
	return f.l.Unlock()
}
