package binarycodec_test

import (
	"errors"
	"math"
	"testing"

	"github.com/huykingsofm/project-b/shared/binarycodec"
)

func TestValuesComeBackAsTheyWereWritten(t *testing.T) {
	var w binarycodec.Writer
	w.Uint8(200)
	for _, v := range []uint64{0, 1, 127, 128, math.MaxUint32, math.MaxUint64} {
		w.Uvarint(v)
	}
	for _, v := range []int64{0, 1, -1, 63, -64, math.MinInt64, math.MaxInt64} {
		w.Varint(v)
	}
	w.Fixed64(0xdeadbeefcafef00d)
	w.String("")
	w.String("héllo\n")

	r := binarycodec.NewReader(w.B)
	if r.Uint8() != 200 {
		t.Fatal("Uint8")
	}
	for _, v := range []uint64{0, 1, 127, 128, math.MaxUint32, math.MaxUint64} {
		if got := r.Uvarint(); got != v {
			t.Fatalf("Uvarint %d, want %d", got, v)
		}
	}
	for _, v := range []int64{0, 1, -1, 63, -64, math.MinInt64, math.MaxInt64} {
		if got := r.Varint(); got != v {
			t.Fatalf("Varint %d, want %d", got, v)
		}
	}
	if got := r.Fixed64(); got != 0xdeadbeefcafef00d {
		t.Fatalf("Fixed64 %x", got)
	}
	if r.String() != "" || r.String() != "héllo\n" {
		t.Fatal("String")
	}
	if r.Err() != nil || r.Left() != 0 {
		t.Fatalf("error %v, %d bytes left", r.Err(), r.Left())
	}
}

func TestSmallNumbersTakeOneByte(t *testing.T) {
	var w binarycodec.Writer
	w.Uvarint(100)
	w.Varint(-60)
	if len(w.B) != 2 {
		t.Fatalf("%d bytes", len(w.B))
	}
}

func TestBytesThatAreCutOffGiveAnErrorAndZeroValuesNeverAPanic(t *testing.T) {
	var w binarycodec.Writer
	w.String("hello")
	w.Fixed64(1)
	full := w.B
	for cut := range full {
		r := binarycodec.NewReader(full[:cut])
		_, _, _, _ = r.String(), r.Fixed64(), r.Uvarint(), r.Uint8()
		if !errors.Is(r.Err(), binarycodec.ErrShort) {
			t.Fatalf("cut at %d: error %v", cut, r.Err())
		}
	}
}

func TestAStringThatClaimsMoreBytesThanThereAreIsRefused(t *testing.T) {
	var w binarycodec.Writer
	w.Uvarint(1 << 40)
	r := binarycodec.NewReader(w.B)
	if s := r.String(); s != "" || !errors.Is(r.Err(), binarycodec.ErrShort) {
		t.Fatalf("got %q, %v", s, r.Err())
	}
}

func TestAnIntegerOfMoreThan64BitsIsABadValue(t *testing.T) {
	r := binarycodec.NewReader(
		[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01},
	)
	if r.Uvarint() != 0 || !errors.Is(r.Err(), binarycodec.ErrBadValue) {
		t.Fatalf("error %v", r.Err())
	}
}

func TestTheFirstErrorIsKept(t *testing.T) {
	r := binarycodec.NewReader(nil)
	r.Uint8()
	r.Uvarint()
	if !errors.Is(r.Err(), binarycodec.ErrShort) {
		t.Fatal(r.Err())
	}
}

func TestABoolComesBackAndAnyOtherByteIsABadValue(t *testing.T) {
	var w binarycodec.Writer
	w.Bool(true)
	w.Bool(false)
	r := binarycodec.NewReader(w.B)
	if !r.Bool() || r.Bool() || r.Err() != nil {
		t.Fatalf("a bool did not come back: %v", r.Err())
	}
	bad := binarycodec.NewReader([]byte{2})
	if bad.Bool(); !errors.Is(bad.Err(), binarycodec.ErrBadValue) {
		t.Fatalf("the byte 2 is not a bool, got %v", bad.Err())
	}
}
