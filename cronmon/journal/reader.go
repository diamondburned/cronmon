package journal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon"
	"github.com/diamondburned/backwardio"
	"github.com/pkg/errors"
)

// Reader implements a primitive reader that can parse journals written by
// Writer from top to bottom.
type Reader struct {
	b *backwardio.Scanner
}

// NewReader creates a new journal reader.
func NewReader(r io.ReadSeeker) *Reader {
	return &Reader{backwardio.NewScanner(r)}
}

// Read reads a single entry, starting from the top file. An EOF error is
// returned if the file has been fully consumed.
func (r *Reader) Read() (cronmon.Event, time.Time, error) {
	var line []byte
	var err error

	for {
		line, err = r.b.ReadUntil('\n')
		if err != nil {
			return nil, time.Time{}, err
		}
		if len(line) > 0 {
			break
		}
	}

	var rawEvent struct {
		Time time.Time       `json:"time"`
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(line, &rawEvent); err != nil {
		return nil, time.Time{}, errors.Wrap(err, "failed to decode JSON")
	}

	event := cronmon.NewEvent(rawEvent.Type)
	if event == nil {
		return nil, time.Time{}, fmt.Errorf("unknown event %q", rawEvent.Type)
	}

	if err := json.Unmarshal(rawEvent.Data, event); err != nil {
		return nil, time.Time{}, errors.Wrap(err, "failed to decode event data")
	}

	return event, rawEvent.Time, nil
}

// ReadPreviousStateFromFile reads the PreviousState from the given file path.
func ReadPreviousStateFromFile(path string) (*cronmon.PreviousState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ReadPreviousState(f)
}

// ReadPreviousState reads backwards the given reader to return the
// PreviousState.
func ReadPreviousState(r io.ReadSeeker) (*cronmon.PreviousState, error) {
	return cronmon.ReadPreviousState(NewReader(r))
}
