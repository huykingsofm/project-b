# R13. Performance: benchmarks are mandatory

After any change that could affect performance or behavior, run the tests, the linter and the relevant benchmarks, compare with the previous numbers, and record them in the PR. Benchmarks are mandatory. Optional optimization features are deferred ([R5](r05-essentials-built-now.md)); a measured cost that is not a design problem is recorded for the profiling step and left alone.
