package journal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon"
	"git.unix.lgbt/diamondburned/cronmon/cronmon/journal/backwardio"
	"github.com/pkg/errors"
)

// Reader implements a primitive reader that can parse journals written by
// Writer from top to bottom.
type Reader struct {
	b *backwardio.BackwardsReader
}

// NewReader creates a new journal reader.
func NewReader(r io.ReadSeeker) *Reader {
	return &Reader{backwardio.NewBackwardsReader(r)}
}

// Read reads a single entry, starting from the top file. An EOF error is
// returned if the file has been fully consumed.
func (r *Reader) Read() (*Event, error) {
	line, err := r.b.ReadUntil('\n')
	if err != nil {
		return nil, err
	}

	var rawEvent struct {
		Time time.Time       `json:"time"`
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(line, &rawEvent); err != nil {
		return nil, errors.Wrap(err, "failed to decode JSON")
	}

	event := cronmon.NewEvent(rawEvent.Type)
	if event == nil {
		return nil, fmt.Errorf("unknown event %q", rawEvent.Type)
	}

	if err := json.Unmarshal(rawEvent.Data, event); err != nil {
		return nil, errors.Wrap(err, "failed to decode event data")
	}

	return &Event{
		Time: rawEvent.Time,
		Type: rawEvent.Type,
		Data: event,
	}, nil
}

// PreviousState parses the last cronmon's previous state.
type PreviousState struct {
	PPID      int
	StartedAt time.Time
}

// ReadPreviousStateFromFile reads the PreviousState from the given file path.
func ReadPreviousStateFromFile(path string) (*PreviousState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ReadPreviousState(f)
}

// ReadPreviousState reads backwards the given reader to return the
// PreviousState.
func ReadPreviousState(r io.ReadSeeker) (*PreviousState, error) {
	reader := NewReader(r)
	state := PreviousState{
		PPID: -1,
	}

	for {
		event, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, io.ErrUnexpectedEOF
			}

			return nil, err
		}

		switch data := event.Data.(type) {
		case *cronmon.EventAcquired:
			state.PPID = data.PPID
			state.StartedAt = event.Time
			return &state, nil
		}
	}
}
