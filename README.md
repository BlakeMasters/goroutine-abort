# Go goroutine abort

Experimental Go 1.26.4 runtime patch for terminating a selected pure-Go
goroutine without terminating its process.

Requires a patched Go toolchain. This is research code, not a production
library or an isolation boundary.

## API

The patched `runtime` package adds `GoroutineHandle` and two modes:

- `GoroutineAbortGraceful` injects `runtime.Goexit`; user defers run.
- `GoroutineAbortHard` destroys the goroutine without running user defers.

Handles include a goroutine ID. A recycled runtime G cannot consume a request
for an earlier incarnation. Hard requests take precedence over graceful
requests.

Hard abort completes only at an asynchronous-preemption boundary classified as
ordinary user Go code. Requests remain pending during channel, timer, syscall,
runtime, C, cgo, and host callback work.

## Limits

Hard abort can strand mutexes, skip cleanup, leak resources, and leave
application state partially changed. It does not provide application-state
recovery or process isolation.

## Status

The Go 1.26.4 Linux/amd64 prototype passed focused tests, selected race tests,
trace parsing, Linux/386 cross-compilation, `go vet`, and upstream `runtime`
tests in a network-disabled container. Results are limited to the tested
runtime version and environment.

See [DESIGN.md](docs/DESIGN.md), [FINDINGS.md](docs/FINDINGS.md), and
[patches/README.md](patches/README.md).

MIT License. See [LICENSE](LICENSE).
