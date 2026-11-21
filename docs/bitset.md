# Bitset

`shared/bitset`. A fixed number of bits. The package imports nothing of the project.

## Array

`Array` is K bits, numbered from 0 to K-1, all clear when made. 1 bit each: K bits take `ceil(K/64)` words of 8 bytes.

| Method | Result |
|---|---|
| `New(k)` | an `*Array` of `k` bits; `ErrNegativeLength` when `k` is below zero |
| `Len()` | `K` |
| `Get(i)` | whether bit `i` is set |
| `Set(i)` | sets bit `i`; reports whether `i` is inside the array |
| `Clear(i)` | clears bit `i`; reports whether `i` is inside the array |
| `Count()` | how many bits are set |

- A bit outside `0..K-1` reads as clear and cannot be changed, and nothing panics.
- Not safe for a writer and a reader at the same time; an `Array` that is no longer changed can be read by many goroutines.

Benchmarks against `[]bool`: [bitset](benchmark/bitset.md).
