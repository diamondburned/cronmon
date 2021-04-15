// Package journal provides an implementation of cromon's Journaler interface to
// write to a file. It also provides a file locking abstraction so that only one
// cronmon instance can run with the same journal file.
package journal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon"
	"github.com/diamondburned/backwardio"
	"github.com/gofrs/flock"
	"github.com/pkg/errors"
)

// multiWriter combines multiple journalers.
type multiWriter struct {
	id      string
	writers []cronmon.Journaler
}

// MultiWriter creates a journaler that writes to multiple other journalers. The
// passed in ID is the one used for the new journaler.
func MultiWriter(ws ...cronmon.Journaler) cronmon.Journaler {
	return wrapMultiWriter(ws...)
}

func wrapMultiWriter(ws ...cronmon.Journaler) *multiWriter {
	id := strings.Builder{}
	id.Grow(256)

	for i, w := range ws {
		id.WriteString(w.ID())

		if i != len(ws)-1 {
			id.WriteByte('+')
		}
	}

	return &multiWriter{id.String(), ws}
}

func (w *multiWriter) ID() string { return w.id }

func (w *multiWriter) Write(event cronmon.Event) error {
	var firstErr error
	for _, writer := range w.writers {
		if err := writer.Write(event); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

type multiReadWriter struct {
	multiWriter
	cronmon.JournalReader
}

// MultiReadWriter creates a journaler that writes to multiple other journalers
// but reads from a single journaler. The new journaler makes no guarantee that
// the reader will read only once all writers are done, so caller should not
// assume that.
func MultiReadWriter(
	r cronmon.JournalReadWriter, ws ...cronmon.Journaler) cronmon.JournalReadWriter {

	return &multiReadWriter{
		multiWriter:   *wrapMultiWriter(append([]cronmon.Journaler{r}, ws...)...),
		JournalReader: r,
	}
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
	Reader
	f *os.File
	l *flock.Flock
}

// ErrLockedElsewhere is returned if NewFileLockJournaler can't acquire the file
// lock.
var ErrLockedElsewhere = errors.New("file already locked elsewhere")

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
	// Ensure the directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, errors.Wrap(err, "failed to create journal directory")
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file")
	}

	l := flock.New(path)

	var locked bool
	if ctx != nil {
		locked, err = l.TryLockContext(ctx, 25*time.Millisecond)
	} else {
		locked, err = l.TryLock()
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to acquire lock")
	}

	if !locked {
		return nil, ErrLockedElsewhere
	}

	return &FileLockJournaler{
		Writer: Writer{json.NewEncoder(f), "file:" + path},
		Reader: Reader{backwardio.NewScanner(f)},
		f:      f,
		l:      l,
	}, nil
}

// Close closes the file and releases the flock.
func (f *FileLockJournaler) Close() error {
	f.f.Close()
	return f.l.Unlock()
}
