# Benchmarks on one core and on more

Conventions and how to run: [index](index.md). Behavior: [cluster](../engine_lifecycle.md).

## Benchmarks

| Name | Package | Measures |
|---|---|---|
| `BenchmarkZoneLayersInParallel/layers=N` | `server/game` | a tick of each of `N` zonelayers of 150 characters each, on a goroutine of its own, as the cluster runs them: the time of one tick of the whole set |
| `BenchmarkStepZoneLayer`, `BenchmarkStepWorld`, `BenchmarkRunScenarioLive`, `BenchmarkReplayCluster`, `BenchmarkRecover` | `server/game` | the benchmarks of [checkpoint](checkpoint.md) and [replay](replay.md) that run on one goroutine, on 1 and on 2 cores, to see that what is not parallel does not get worse |

- The cores are set in two ways: `-cpu N` gives the program `N` for `GOMAXPROCS`, and `taskset -c 0` or `taskset -c 0,1` before `go test` leaves the program the cores it names, whatever it asks for. The tables say which.
- The machine has 12 threads; the runs of the first table leave the other cores to the system.

## Results

The time of one tick of all the zonelayers (µs), with `-cpu`:

| Zonelayers | 1 CPU | 2 CPUs | 4 CPUs | Gain of 4 CPUs over 1 |
|---|---|---|---|---|
| 1 | 37.4 | 32.9 | 42.9 | none |
| 2 | 78.9 | 64.6 | 73.5 | 1.1 times |
| 4 | 177.9 | 143.4 | 137.8 | 1.3 times |
| 8 | 353.8 | 242.0 | 189.9 | 1.9 times |

The same with `taskset`, which leaves 1 core or 2 (µs):

| Zonelayers | 1 core | 2 cores | Gain |
|---|---|---|---|
| 1 | 35.1 | 32.4 | 1.1 times |
| 2 | 74.8 | 68.3 | 1.1 times |
| 4 | 168.4 | 146.7 | 1.1 times |
| 8 | 355.9 | 263.4 | 1.4 times |

What runs on one goroutine, on 1 and on 2 CPUs (`-cpu`):

| Benchmark | 1 CPU | 2 CPUs |
|---|---|---|
| StepZoneLayer | 44.1 µs | 41.4 µs |
| StepWorld | 203.8 µs | 262.9 µs |
| RunScenarioLive | 1,171 µs | 995 µs |
| ReplayCluster | 1,492 µs | 1,299 µs |
| Recover | 19.8 ms | 18.6 ms |

## Reading

- **On one core the layers add up:** 44 µs for each, the time of `StepZoneLayer`, so 8 layers are 354 µs. That is the cost of a cluster that has one core to run on.
- **More cores help less than the number of them:** 8 layers go from 354 to 242 µs with 2 CPUs (1.5 times) and to 190 µs with 4 (1.9 times). A tick of a zonelayer allocates 85 KB in 647 allocations, and the allocator and the collector are shared by the goroutines, so the gain is probably bound by memory and not by the work; this was not checked with a profile.
- **One layer does not gain,** as it is one goroutine; the 43 µs with 4 CPUs against 37 with 1 is the spread between runs.
- **What does not run in parallel does not get worse with a second core:** the other benchmarks are within the spread of the runs (`StepWorld` is the one that differs, 204 and 263 µs, and its spread between runs is of the same size).
- The runs of `-cpu` and of `taskset` differ by up to 25 percent for the same number of cores, which is the spread of this machine (WSL2) and not an effect of the way the cores are set.
