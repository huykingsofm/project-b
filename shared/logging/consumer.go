package logging

import (
	"io"
	"sync"
)

// Consumer takes the entries of the log out of memory. Init calls Consume on a goroutine of its
// own, so a Consumer is never called by two goroutines at a time.
type Consumer interface {
	// Consume handles a batch of entries, in the order they were made. An error is reported on
	// stderr and the batch is not offered again.
	Consume(entries []Entry) error
}

// writerConsumer encodes entries as lines and writes a batch with one Write.
type writerConsumer struct {
	mu  sync.Mutex
	w   io.Writer
	buf []byte
}

// NewWriterConsumer returns a Consumer that writes every batch to w as lines of key=value pairs,
// for example os.Stderr or a file.
func NewWriterConsumer(w io.Writer) Consumer { return &writerConsumer{w: w} }

func (c *writerConsumer) Consume(entries []Entry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = c.buf[:0]
	for _, e := range entries {
		c.buf = appendEntry(c.buf, e)
	}
	_, err := c.w.Write(c.buf)
	return err
}
