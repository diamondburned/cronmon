package backwardio

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestBackwardsReader(t *testing.T) {
	maxTok = 3
	t.Cleanup(func() {
		maxTok = bufio.MaxScanTokenSize
	})

	type test struct {
		name   string
		input  string
		output []string
	}

	var tests = []test{
		{"enough", "aa\nbb\ncc\ndd\n", []string{"", "dd", "cc", "bb", "aa"}},
		{"enough both", "\naa\nbb\n", []string{"", "bb", "aa", ""}},
		{"enough prefix", "\naa\nbb", []string{"bb", "aa", ""}},

		{"short", "a\nb\nc\nd\n", []string{"", "d", "c", "b", "a"}},
		{"short both", "\na\nb\n", []string{"", "b", "a", ""}},
		{"short prefix", "\na\nb", []string{"b", "a", ""}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := NewBackwardsReader(strings.NewReader(test.input))

			for _, expect := range test.output {
				b, err := r.ReadUntil('\n')
				if err != nil {
					t.Fatal("failed to read:", err)
				}

				s := string(b)

				if s != expect {
					t.Errorf("expected %q, got %q", expect, s)
				}
			}

			_, err := r.ReadUntil('\n')
			errorEq(t, err, io.EOF)
		})
	}

	t.Run("too long", func(t *testing.T) {
		const input = "aaaaa\nbbbbb"

		r := NewBackwardsReader(strings.NewReader(input))

		_, err := r.ReadUntil('\n')
		errorEq(t, err, bufio.ErrTooLong)
	})
}

func TestBackwardsReaderError(t *testing.T) {
	// For the sake of 100% coverage, we'll test if the code returns the right
	// error when we mimic certain failing behaviors of io.ReadSeeker.

	fseek := failSeeker{
		err: errors.New("custom error"),
	}

	type seekError struct {
		name  string
		error string
	}

	seekErrors := []seekError{
		// Keep these in sync with fill()'s implementation.
		{"seek end", "failed to find end of file"},
		{"seek start", "failed to seek backwards"},
		{"read", "failed to read seeked chunk"},
	}

	for i, seekErr := range seekErrors {
		t.Run(seekErr.name, func(t *testing.T) {
			fseek.stage = i
			r := NewBackwardsReader(fseek)

			_, err := r.ReadUntil(0)
			errorEq(t, err, fseek.err)

			if !strings.Contains(err.Error(), seekErr.error) {
				t.Fatalf("returned error does not contain substring\n"+
					"got:      %q\n"+
					"expected: %q", err, seekErr.error)
			}
		})
	}
}

func errorEq(t *testing.T, got, expect error) {
	t.Helper()

	if got == nil {
		t.Fatal("missing error")
	}

	if !errors.Is(got, expect) {
		t.Fatal("unexpected error:", got)
	}
}

type failSeeker struct {
	err   error
	stage int
}

var _ io.ReadSeeker = (*failSeeker)(nil)

func (s failSeeker) Read(b []byte) (int, error) {
	if s.stage == 2 {
		return 0, s.err
	}

	return len(b), nil
}

func (s failSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekEnd:
		if s.stage == 0 {
			return 0, s.err
		}
		return 10, nil

	case io.SeekStart:
		if s.stage == 1 {
			return 0, s.err
		}
		return offset, nil

	case io.SeekCurrent:
		return 0, errors.New("cannot handle io.SeekCurrent")
	default:
		return 0, errors.New("unknown whence value")
	}
}
