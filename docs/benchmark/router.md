# Benchmarks of the router

Conventions and how to run: [index](index.md). Behavior: [delivery of messages](../simulation.md#delivery-of-messages).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkDeliver` | `platform/runtime/router` | the send of one message to an engine, and the read of it by that engine |
| `BenchmarkDeliverFromManyEngines` | `platform/runtime/router` | the same as the first, from as many goroutines as the run has CPUs, each an engine with its own channel in the one router |
| `BenchmarkReturn` | `platform/runtime/router` | the return of a message to a sender that is there, and the read of it |

- Run with `-cpu 1,2,4`: the last one is the one that depends on it. `ns/op` of a parallel run is the time of the whole run divided by the operations of every goroutine, so a lower number with more CPUs is a gain.

## Results

| Benchmark | CPUs | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| Deliver | 1 | 30.4 | 0 | 0 |
| Deliver | 2 | 30.4 | 0 | 0 |
| Deliver | 4 | 30.7 | 0 | 0 |
| DeliverFromManyEngines | 1 | 29.6 | 0 | 0 |
| DeliverFromManyEngines | 2 | 23.6 | 0 | 0 |
| DeliverFromManyEngines | 4 | 19.1 | 0 | 0 |
| Return | 1 | 30.5 | 0 | 0 |
| Return | 2 | 30.4 | 0 | 0 |
| Return | 4 | 30.2 | 0 | 0 |

## Reading

- **A send is 30 ns and allocates nothing:** the table of channels is read without a lock, and the channel takes the message without waiting; most of the 30 ns is the send and the read on the channel.
- **From many engines the time of one send falls from 30 to 19 ns with 4 CPUs,** since each engine has a channel of its own and the router holds no lock: more engines do not make one wait for another. A run of several senders to one receiver would wait on its channel, and is not measured here.
- A tick of an engine is 40 µs to 0.2 ms, so a message costs a thousandth of it.
