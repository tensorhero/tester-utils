package linewriter

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// syncedBuffer is a thread-safe bytes.Buffer for use in tests where a
// background goroutine writes while the test goroutine reads.
type syncedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncedBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncedBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestWritesLine(t *testing.T) {
	w := &syncedBuffer{}
	lw := New(w, 100*time.Millisecond)
	n, _ := lw.Write([]byte{'a', 'b', 'c', '\n'})
	if !assert.Equal(t, 4, n) {
		t.FailNow()
	}

	err := lw.Flush()
	if !assert.Nil(t, err) {
		t.FailNow()
	}

	if !assert.Equal(t, "abc\n", w.String()) {
		t.FailNow()
	}
}

func TestWritesNewLine(t *testing.T) {
	w := &syncedBuffer{}
	lw := New(w, 100*time.Millisecond)
	n, _ := lw.Write([]byte{'a', 'b', 'c', '\n', '\n'})
	if !assert.Equal(t, 5, n) {
		t.FailNow()
	}

	err := lw.Flush()
	if !assert.Nil(t, err) {
		t.FailNow()
	}

	if !assert.Equal(t, "abc\n\n", w.String()) {
		t.FailNow()
	}
}

func TestHandlesTimeout(t *testing.T) {
	w := &syncedBuffer{}
	lw := New(w, 100*time.Millisecond)
	n, _ := lw.Write([]byte{'a', 'b', 'c'})
	if !assert.Equal(t, 3, n) {
		t.FailNow()
	}

	if !assert.Equal(t, "", w.String()) {
		t.FailNow()
	}

	time.Sleep(200 * time.Millisecond)

	if !assert.Equal(t, "abc\n", w.String()) {
		t.FailNow()
	}

	lw.Write([]byte{'a'})

	if !assert.Equal(t, "abc\n", w.String()) {
		t.FailNow()
	}

	time.Sleep(200 * time.Millisecond)

	if !assert.Equal(t, "abc\na\n", w.String()) {
		t.FailNow()
	}
}
