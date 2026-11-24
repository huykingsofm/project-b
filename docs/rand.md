# Random numbers

`shared/rand`. The random numbers of one owner, such as an engine, with their place written to bytes and read back, so that a run goes on with the numbers it would have drawn. It is the PCG of `math/rand/v2` and the methods of its `Rand`. It imports of the project only `shared/codec/binary`.

| Call | What it does |
|---|---|
| `rand.NewPCG(seed1, seed2)` | makes the random numbers of a PCG that the two seeds decide; the same seeds give the same numbers |
| the methods of `math/rand/v2`'s `Rand` | `IntN`, `Float64`, `Shuffle`, ... : `rand.Rand` holds a `*rand.Rand` of the standard package, so they are its methods |
| `Encode(w)` | writes where the PCG is, as bytes with their length |
| `rand.Decode(r)` | reads what `Encode` wrote: random numbers that go on from that place; an error for bytes that are not a place |

- The engine makes its random numbers from the seed of the cluster and its own ID (`rand.NewPCG(seed, uint64(id))`), writes them in its checkpoint, and gives the standard `Rand` to its actions (`Context.Rand`).
- A checkpoint holds the place and not the seeds, and a replay that starts from it draws what the first run drew after it.
