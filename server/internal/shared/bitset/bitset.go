// Package bitset holds a fixed number of bits. It imports nothing of the project.
package bitset

import (
	"errors"
	"math/bits"
)

// ErrNegativeLength is returned by New for a length below zero.
var ErrNegativeLength = errors.New("bitset: the length cannot be negative")

// Array is K bits, all clear when it is made, numbered from 0 to K-1. A bit outside that range reads as
// clear and cannot be changed, so a wrong index never panics.
type Array struct {
	words []uint64
	k     int
}

// New makes an Array of k bits.
func New(k int) (*Array, error) {
	if k < 0 {
		return nil, ErrNegativeLength
	}
	return &Array{words: make([]uint64, (k+63)/64), k: k}, nil
}

// Len is the number of bits, K.
func (a *Array) Len() int { return a.k }

// Get reports whether bit i is set; a bit outside the array is not.
func (a *Array) Get(i int) bool {
	if uint(i) >= uint(a.k) {
		return false
	}
	return a.words[i>>6]&(1<<(uint(i)&63)) != 0
}

// Set sets bit i and reports whether i is inside the array.
func (a *Array) Set(i int) bool {
	if uint(i) >= uint(a.k) {
		return false
	}
	a.words[i>>6] |= 1 << (uint(i) & 63)
	return true
}

// Clear clears bit i and reports whether i is inside the array.
func (a *Array) Clear(i int) bool {
	if uint(i) >= uint(a.k) {
		return false
	}
	a.words[i>>6] &^= 1 << (uint(i) & 63)
	return true
}

// Count is the number of bits that are set.
func (a *Array) Count() int {
	n := 0
	for _, w := range a.words {
		n += bits.OnesCount64(w)
	}
	return n
}
