# Benchmarks of the bitset

Conventions and how to run: [index](index.md). Behavior: [bitset](../shared.md#bitset).

## Benchmarks

Each one runs on `bitset.Array` and on `[]bool` of the same size, filled the same way (about one bit in four set).

| Name | Package | Measures |
|---|---|---|
| `BenchmarkGetSequential` | `shared/bitset` | `Get` of the indexes 0, 1, 2, ... wrapping at the size, counting the set bits |
| `BenchmarkGetRandom` | `shared/bitset` | `Get` of random indexes, counting the set bits |
| `BenchmarkSetRandom` | `shared/bitset` | `Set` of random indexes |
| `BenchmarkAnyInRange` | `shared/bitset` | looking for a set bit in a range of 1, 5, 64 and 1,000 bits that has none, in a 1,048,576-bit array: `AnyInRange` against a `Get` of each bit, and a `[]bool` read the same way |
| `BenchmarkGetMaskTable` | `shared/bitset` | `Get` with the mask shifted for (`1 << (i & 63)`, what `Array` does) against the mask read from a table of the 64 masks |

- Sizes in bits: 65,536 (8 KiB as bits, 64 KiB as bools), 1,048,576 (128 KiB, 1 MiB), 16,777,216 (2 MiB, 16 MiB; a map of 4096 by 4096 tiles).
- `Array.Get` checks the index and returns clear outside the array; `[]bool` has only the check of the language, which panics.

## Results

Time per access, in ns/op; none allocates.

| Benchmark | bits | bitset | bool |
|---|---|---|---|
| GetSequential | 65,536 | 1.63 | 0.47 |
| GetSequential | 1,048,576 | 2.12 | 0.47 |
| GetSequential | 16,777,216 | 2.14 | 0.47 |
| GetRandom | 65,536 | 0.80 | 0.49 |
| GetRandom | 1,048,576 | 0.82 | 0.68 |
| GetRandom | 16,777,216 | 1.12 | 3.26 |
| SetRandom | 65,536 | 1.23 | 0.80 |
| SetRandom | 1,048,576 | 1.15 | 0.85 |
| SetRandom | 16,777,216 | 1.39 | 2.87 |

Mask from a table of 64, ns/op:

| bits | order | shift | table |
|---|---|---|---|
| 65,536 | sequential | 1.64 | 1.57 |
| 65,536 | random | 0.78 | 0.91 |
| 1,048,576 | sequential | 2.13 | 2.14 |
| 1,048,576 | random | 0.80 | 0.94 |
| 16,777,216 | sequential | 2.15 | 2.15 |
| 16,777,216 | random | 1.11 | 1.23 |

Looking for a set bit in a range that has none, ns/op:

| range in bits | `AnyInRange` | `Get` of each | `[]bool` of each |
|---|---|---|---|
| 1 | 3.24 | 1.46 | 1.06 |
| 5 | 3.21 | 4.09 | 3.00 |
| 64 | 3.95 | 43.2 | 31.0 |
| 1,000 | 11.1 | 666 | 365 |
