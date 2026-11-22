// Package binarycodec writes and reads values as bytes: variable-length integers, fixed-width
// integers and strings. A Reader never panics on bytes that are cut off or wrong: it keeps the
// first error and returns zero values after it, so a decoder reads everything it needs and looks at
// Err once. It imports nothing of the project.
package binarycodec

import (
	"encoding/binary"
	"errors"
)

// ErrShort is the error of a Reader that was asked for more bytes than there are.
var ErrShort = errors.New("binarycodec: the bytes end before the value does")

// ErrBadValue is the error of a Reader that found bytes that are not a value of the kind asked for.
var ErrBadValue = errors.New("binarycodec: the bytes are not a value of that kind")

// Version is the version of the data that is read: how its bytes were encoded. The container of the
// bytes (a segment header, a checkpoint header) says which, and a Decode function is given it, so
// that it can read what an older version wrote.
type Version uint16

// Writer appends values to a slice of bytes.
type Writer struct{ B []byte }

// Uint8 appends one byte.
func (w *Writer) Uint8(v uint8) { w.B = append(w.B, v) }

// Bool appends one byte: 1 for true and 0 for false.
func (w *Writer) Bool(v bool) {
	if v {
		w.B = append(w.B, 1)
		return
	}
	w.B = append(w.B, 0)
}

// Uvarint appends an unsigned integer in 1 to 10 bytes; small values take one.
func (w *Writer) Uvarint(v uint64) { w.B = binary.AppendUvarint(w.B, v) }

// Varint appends a signed integer in 1 to 10 bytes; values near zero take one.
func (w *Writer) Varint(v int64) { w.B = binary.AppendVarint(w.B, v) }

// Fixed64 appends an unsigned integer in 8 bytes, for values that are spread over all 64 bits, like
// a hash.
func (w *Writer) Fixed64(v uint64) { w.B = binary.LittleEndian.AppendUint64(w.B, v) }

// String appends the length of the string and then its bytes.
func (w *Writer) String(s string) {
	w.Uvarint(uint64(len(s)))
	w.B = append(w.B, s...)
}

// Reader reads values from bytes.
type Reader struct {
	b   []byte
	err error
}

// NewReader makes a Reader of b.
func NewReader(b []byte) *Reader { return &Reader{b: b} }

// Err is the first error, or nil.
func (r *Reader) Err() error { return r.err }

// Left is how many bytes have not been read.
func (r *Reader) Left() int { return len(r.b) }

func (r *Reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
	r.b = nil
}

// Uint8 reads one byte.
func (r *Reader) Uint8() uint8 {
	if len(r.b) < 1 {
		r.fail(ErrShort)
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

// Bool reads what Writer.Bool wrote. A byte that is neither 0 nor 1 is a bad value.
func (r *Reader) Bool() bool {
	switch r.Uint8() {
	case 0:
		return false
	case 1:
		return true
	}
	r.fail(ErrBadValue)
	return false
}

// Uvarint reads what Writer.Uvarint wrote.
func (r *Reader) Uvarint() uint64 {
	v, n := binary.Uvarint(r.b)
	if n == 0 {
		r.fail(ErrShort)
		return 0
	}
	if n < 0 {
		r.fail(ErrBadValue)
		return 0
	}
	r.b = r.b[n:]
	return v
}

// Varint reads what Writer.Varint wrote.
func (r *Reader) Varint() int64 {
	v, n := binary.Varint(r.b)
	if n == 0 {
		r.fail(ErrShort)
		return 0
	}
	if n < 0 {
		r.fail(ErrBadValue)
		return 0
	}
	r.b = r.b[n:]
	return v
}

// Fixed64 reads what Writer.Fixed64 wrote.
func (r *Reader) Fixed64() uint64 {
	if len(r.b) < 8 {
		r.fail(ErrShort)
		return 0
	}
	v := binary.LittleEndian.Uint64(r.b)
	r.b = r.b[8:]
	return v
}

// String reads what Writer.String wrote.
func (r *Reader) String() string {
	n := r.Uvarint()
	if r.err != nil {
		return ""
	}
	if n > uint64(len(r.b)) {
		r.fail(ErrShort)
		return ""
	}
	s := string(r.b[:n])
	r.b = r.b[n:]
	return s
}
