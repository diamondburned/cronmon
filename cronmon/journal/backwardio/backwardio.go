// Package backwardio implements a buffered scanner that scans backwards.
package backwardio

import (
	"bufio"
	"io"

	"github.com/pkg/errors"
)

var maxTok = bufio.MaxScanTokenSize

// BackwardsReader is a reader that reads backwards, similar to bufio except
// things are scanned backwards.
type BackwardsReader struct {
	r   io.ReadSeeker
	buf []byte
	end int64 // last seeked, bound size for buf
}

func NewBackwardsReader(r io.ReadSeeker) *BackwardsReader {
	return &BackwardsReader{r: r}
}

// ReadUntil reads until a new line is encountered.
func (r *BackwardsReader) ReadUntil(delim byte) ([]byte, error) {
	for {
		if r.buf == nil {
			goto fill
		}

		// Seek backwards the buffer until we find a delimiter.
		for i := len(r.buf) - 1; i >= 0; i-- {
			isBOF := i == 0 && r.end == 0

			// If the current byte is not a delimiter AND we have not consumed
			// the whole reader yet, then skip.
			if r.buf[i] != delim && !isBOF {
				continue
			}

			tok := r.buf[i:]
			r.buf = r.buf[:i]

			if len(tok) > 0 && tok[0] == '\n' {
				tok = tok[1:] // trim prefix delim

				// If this is the beginning of file and we have a prefixing new
				// line, then we should make that its own token. If the token is
				// already a new line, then bail.
				if isBOF && len(tok) > 0 {
					r.buf = r.buf[:1]
				}
			}

			return tok, nil
		}

		if len(r.buf) == cap(r.buf) {
			// At this point, we started from the end of the buffer and read all
			// the way until the start of the buffer, and we couldn't find the
			// delimiter. Filling up further won't do anything.
			return nil, bufio.ErrTooLong
		}

	fill:
		if err := r.fill(); err != nil {
			return nil, err
		}
	}
}

func (r *BackwardsReader) fill() error {
	if r.buf == nil {
		o, err := r.r.Seek(0, io.SeekEnd)
		if err != nil {
			return errors.Wrap(err, "failed to find end of file")
		}

		r.end = o
		r.buf = make([]byte, 0, maxTok)
	}

	if r.end == 0 {
		return io.EOF
	}

	// Try to see how much we can actually read into the buffer.
	max := int64(cap(r.buf))

	if len(r.buf) > 0 {
		// Subtract the read bounds by the cursor position, since that end
		// region is going to be reserved for old data.
		max -= int64(len(r.buf))
		// Grow the buffer to its maximum capacity.
		r.buf = r.buf[:cap(r.buf)]
		// Copy what we've already read into the end of the buffer.
		copy(r.buf[max:], r.buf)
	}

	seekTo := r.end - max
	min := int64(0)

	// If we've seeked to the start of the file, then what we're about to read
	// may not fill up all of our buffer. Thus, we need to know the offset
	// relative to the last seeked position and use that as the starting bound.
	if seekTo < 0 {
		seekTo = 0
		min = max - r.end
	}

	// Seek backwards before reading forward. We want to use the capacity of
	// the buffer instead of the length so we can slice it off later.
	_, err := r.r.Seek(seekTo, io.SeekStart)
	if err != nil {
		return errors.Wrap(err, "failed to seek backwards")
	}

	r.end = seekTo

	// Read the seeked chunk.
	_, err = r.r.Read(r.buf[min:max])
	if err != nil {
		return errors.Wrap(err, "failed to read seeked chunk")
	}

	// Set the buffer to only the valid chunk.
	r.buf = r.buf[min:cap(r.buf)]

	return nil
}
