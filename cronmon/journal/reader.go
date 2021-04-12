package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"git.unix.lgbt/diamondburned/cronmon/cronmon"
	"github.com/pkg/errors"
)

// Reader implements a primitive reader that can parse journals written by
// Writer from top to bottom.
type Reader struct {
	b *bufio.Reader
}

// NewReader creates a new journal reader.
func NewReader(r io.ReadSeeker) (*Reader, error) {

	reader := io.LimitReader(r, o)
	return &Reader{bufio.NewReader(reader)}, nil
}

// Read reads a single entry, starting from the top file.
func (r *Reader) Read() (*Event, error) {
	line, err := r.b.ReadSlice('\n')
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
