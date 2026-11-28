# Small shared libraries

Three libraries of `shared` that import nothing of the project but `shared` itself: `atomicfile`, `rand` and `bitset`. The larger ones have a document of their own: [logging](logging.md) and [segmentlog](segmentlog.md); the others are listed in [simulation](simulation.md#shared-libraries-and-configuration).

## atomicfile

`shared/atomicfile`. Writes a file so that it is on the disk whole or not there at all, and syncs directories. It imports nothing of the project.

### Entry points

| Call | What it does |
|---|---|
| `atomicfile.Write(path, data)` | makes the directory of `path` when it is not there, writes `path + ".tmp"`, syncs it, renames it to `path`, and syncs the directory; a file already at `path` is replaced whole |
| `atomicfile.Stamped(name, now)` | the name of a file put aside: `<origin>.<now>.<ext>`, with `now` in UTC to the nanosecond (`20060102T150405` + 9 digits + `Z`), so that a file put aside before under the same name is not written over |
| `atomicfile.SyncDir(dir)` | forces the entries of a directory to disk, so that a file made, renamed or deleted in it is there after a crash |

- A temporary file that a crash left is not the file; the next `Write` of the same path writes over it.
- Used by the checkpoint files ([checkpoint](checkpoint.md)), the cache of the recovery line ([replay](replay.md)) and the segment log ([segmentlog](segmentlog.md)).

## rand

`shared/rand`. The random numbers of one owner, such as an engine, with their place written to bytes and read back, so that a run goes on with the numbers it would have drawn. It is the PCG of `math/rand/v2` and the methods of its `Rand`. It imports of the project only `shared/codec/binary`.

| Call | What it does |
|---|---|
| `rand.NewPCG(seed1, seed2)` | makes the random numbers of a PCG that the two seeds decide; the same seeds give the same numbers |
| the methods of `math/rand/v2`'s `Rand` | `IntN`, `Float64`, `Shuffle`, ... : `rand.Rand` holds a `*rand.Rand` of the standard package, so they are its methods |
| `Encode(w)` | writes where the PCG is, as bytes with their length |
| `rand.Decode(r)` | reads what `Encode` wrote: random numbers that go on from that place; an error for bytes that are not a place |

- The engine makes its random numbers from the seed of the cluster and its own ID (`rand.NewPCG(seed, uint64(id))`), writes them in its checkpoint, and gives the standard `Rand` to its actions (`Context.Rand`).
- A checkpoint holds the place and not the seeds, and a replay that starts from it draws what the first run drew after it.

## bitset

`shared/bitset`. A fixed number of bits. The package imports nothing of the project.

### Array

`Array` is K bits, numbered from 0 to K-1, all clear when made. 1 bit each: K bits take `ceil(K/64)` words of 8 bytes.

| Method | Result |
|---|---|
| `New(k)` | an `*Array` of `k` bits; `ErrNegativeLength` when `k` is below zero |
| `Len()` | `K` |
| `Get(i)` | whether bit `i` is set |
| `Set(i)` | sets bit `i`; reports whether `i` is inside the array |
| `Clear(i)` | clears bit `i`; reports whether `i` is inside the array |
| `Count()` | how many bits are set |
| `AnyInRange(lo, n)` | whether any of the `n` bits from `lo` is set; the part outside the array is clear; one word read for each 64 bits |
| `Words()`, `FromWords(k, words)` | the bits as the words they are kept in (a copy), and the array made from them; `ErrWords` when the number of words is not the one `k` bits take, and the bits past `k` are cleared |
| `Clone()` | a copy that shares nothing with the array |

- `AnyInRange` takes about 3 ns for any short range, so a range of 1 to 3 bits is cheaper read with `Get`.
- A bit outside `0..K-1` reads as clear and cannot be changed, and nothing panics.
- Not safe for a writer and a reader at the same time; an `Array` that is no longer changed can be read by many goroutines.

Benchmarks against `[]bool`: [bitset](benchmark/bitset.md).
