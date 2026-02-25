package linewriter

import (
	"bytes"
	"io"
	"time"
)

type LineWriter struct {
	buffer      chan byte
	writer      io.Writer
	timeout     time.Duration
	lastErr     error
	flushChan   chan bool
	flushedChan chan bool
}

// Write queues a string for writing
func (w *LineWriter) Write(bytes []byte) (n int, err error) {
	for _, byte := range bytes {
		w.buffer <- byte
	}

	return len(bytes), nil
}

// Flush flushes any pending strings, and returns an error if any writes failed
// in the past
func (w *LineWriter) Flush() (err error) {
	w.flushChan <- true
	<-w.flushedChan
	return w.lastErr
}

// New returns a LineWriter instance
func New(w io.Writer, timeout time.Duration) *LineWriter {
	lw := &LineWriter{
		buffer:      make(chan byte),
		writer:      w,
		timeout:     timeout,
		flushChan:   make(chan bool),
		flushedChan: make(chan bool),
	}
	lw.startWriter()
	return lw
}

// startWriter runs a single goroutine that accumulates bytes, flushes on newline
// or timeout, and handles explicit Flush() calls. All writes to w.writer happen
// in this one goroutine, eliminating concurrent-write races.
func (w *LineWriter) startWriter() {
	go func(w *LineWriter) {
		accumulated := bytes.NewBuffer([]byte{})

		flushAccumulated := func() {
			if accumulated.Len() > 0 {
				_, err := w.writer.Write(accumulated.Bytes())
				if err != nil {
					w.lastErr = err
				}
				accumulated = bytes.NewBuffer([]byte{})
			}
		}

	loop:
		for {
			select {
			case b := <-w.buffer:
				accumulated.WriteByte(b)
				if b == '\n' {
					flushAccumulated()
				}
			case <-time.After(w.timeout):
				if accumulated.Len() > 0 {
					accumulated.WriteByte('\n')
					flushAccumulated()
				}
			case <-w.flushChan:
				if accumulated.Len() > 0 {
					accumulated.WriteByte('\n')
					flushAccumulated()
				}
				w.flushedChan <- true
				break loop
			}
		}
	}(w)
}
