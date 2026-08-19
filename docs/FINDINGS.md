# Findings

## Outcome

The experiment produced a working Linux/amd64 Go 1.26.4 runtime fork that can
hard-abort an exact pure-Go goroutine while the process and runtime continue.
The complete contained acceptance run passed.

## What failed first

### Scheduler-handoff destruction

The first implementation consumed an abort whenever the target entered
`execute`. It passed tight-loop and defer tests, but recycled Gs retained
asynchronous extended-register state and later crashed during preemption.

Clearing that state exposed a deeper failure: stack-shrink state survived G
destruction. Clearing both still deadlocked after hundreds of allocation-heavy
aborts. A goroutine had been erased while participating in GC startup, leaving
all subsequent GC starters waiting on a semaphore with no owner.

Conclusion: a Go async safe point is safe to pause and scan, but a generic
scheduler handoff is not sufficient evidence that a G can be erased.

### Scheduler re-arm livelock

The first stricter design re-applied `stackPreempt` whenever a requested G was
scheduled. A channel-blocked target became runnable but was synchronously
preempted again before completing channel bookkeeping or returning to user
code. Even natural function return was starved.

Conclusion: scheduler progress and abort discovery need separate machinery.
The scheduler now leaves pending requests passive; `sysmon` retries targeted
asynchronous signals while the G is actually running.

### Missing lifecycle events

Directly invoking `goexit0` preserved the process but produced a malformed Go
execution trace: the parser reported `expected no goroutine but had one`.

Conclusion: hard abort must preserve runtime lifecycle accounting. The current
path emits race and trace end state from g0 before destroying the G, while still
skipping user defers.

## Final contained evidence

The final 2026-08-18 run used Go 1.26.4 from the exact
`golang:1.26.4-bookworm` content ID recorded in the README, with networking
disabled after the one-module preparation step.

Passed gates:

- complete focused test package;
- 1,000 hard aborts under allocation churn and forced GC;
- concurrent callers and stale-handle churn;
- `LockOSThread` cleanup and post-abort reuse;
- abort requested during a blocking pipe read, completed only after syscall
  return;
- expected proof that hard abort skips defers and can strand a user mutex;
- 20 repeated basic tests;
- 2,000-cycle latency probe;
- execution trace generation and strict parser acceptance;
- selected race-detector tests;
- Linux/386 cross-compilation;
- `go vet ./...`;
- upstream `go test runtime -count=1`.

Measured probe results:

| Mode | Completed | p50 | p95 | max |
|---|---:|---:|---:|---:|
| Graceful | 1,000/1,000 | 7.357 µs | 22.530 µs | 59.180 µs |
| Hard | 1,000/1,000 | 6.391 µs | 21.533 µs | 128.900 µs |

The single-P allocation/GC churn test took 135.24 seconds for 1,000 hard
aborts. That is not representative throughput, but it proves that availability
of an eligible user-code safe point can dominate completion latency.

## Conclusion

The prototype terminates a selected pure-Go goroutine without application
cooperation under the tested conditions. Hard abort can strand user locks,
transactions, protocols, and partial application mutations. Runtime integrity
does not provide application-state recovery.

Use this repository for runtime research and reproduction. It does not replace
process, Wasm, container, or VM isolation for untrusted or failure-prone work.

## Upstream context

- [Go's asynchronous-preemption implementation](https://go.dev/src/runtime/preempt.go)
  defines safe points as places where execution can be paused and roots can be
  scanned; this experiment found that erasure needs a narrower interpretation.
- [Proposal #25664](https://github.com/golang/go/issues/25664) asked for a
  stoppable goroutine API, including the difficult C-call case.
- [Proposal #50678](https://github.com/golang/go/issues/50678) asked for
  non-cooperative interruption of goroutines that ignore cancellation.
- [Issue #29226](https://github.com/golang/go/issues/29226) documents subtle
  interactions between `Goexit`, deferred panics, and recovery, reinforcing why
  graceful and hard modes should remain distinct.
