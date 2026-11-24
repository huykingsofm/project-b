// Package rand is the random numbers of one owner, such as an engine, with their place written to
// bytes and read back, so that a run goes on with the numbers it would have drawn. It is the PCG of
// the package math/rand/v2 and the methods of its Rand, and it imports of the project only the
// binary codec.
package rand

import (
	"fmt"
	stdrand "math/rand/v2"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Rand draws numbers with the methods of the Rand of math/rand/v2 (IntN, Float64, Shuffle, ...)
// from a PCG, and writes where the PCG is. Make one with NewPCG or Decode.
type Rand struct {
	*stdrand.Rand
	pcg *stdrand.PCG
}

// NewPCG makes the random numbers of a PCG that two seeds decide: the same seeds give the same
// numbers.
func NewPCG(seed1, seed2 uint64) *Rand {
	pcg := stdrand.NewPCG(seed1, seed2)
	return &Rand{Rand: stdrand.New(pcg), pcg: pcg}
}

// Clone returns random numbers that share nothing with these and go on from the same place: the
// same numbers, drawn apart.
func (r *Rand) Clone() *Rand {
	pcg := *r.pcg
	return &Rand{Rand: stdrand.New(&pcg), pcg: &pcg}
}

// Encode writes where the PCG is, as bytes with their length.
func (r *Rand) Encode(w *binary.Writer) error {
	place, err := r.pcg.MarshalBinary()
	if err != nil {
		return err
	}
	w.String(string(place))
	return nil
}

// Decode reads what Encode wrote: random numbers that go on from the place that was written.
func Decode(r *binary.Reader) (*Rand, error) {
	place := r.String()
	if err := r.Err(); err != nil {
		return nil, err
	}
	rng := NewPCG(0, 0)
	if err := rng.pcg.UnmarshalBinary([]byte(place)); err != nil {
		return nil, fmt.Errorf("the place of the random numbers: %w", err)
	}
	return rng, nil
}
