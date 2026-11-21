# R13. Performance: benchmarks are mandatory

After any change that could affect performance or behavior, run the tests, the linter and the relevant benchmarks, compare with the previous numbers, and replace the numbers in `docs/benchmark/` in place, a file for each group of benchmarks. The file always shows the latest run, and its history is in git. Benchmarks are mandatory. Optional optimization features are deferred ([R5](r05-essentials-built-now.md)); a measured cost that is not a design problem is recorded for the profiling step and left alone.
