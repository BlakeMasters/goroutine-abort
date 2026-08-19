# Design

## Objective

Prove or falsify whether remote goroutine termination can be implemented inside
the Go runtime without relying on application cooperation, OS-thread
cancellation, or `unsafe` stack mutation in user code.

The experiment deliberately separates two questions:

1. Can the runtime identify and eventually stop one exact goroutine incarnation?
2. Can doing so preserve the target process and Go runtime, even though the
   application state owned by that goroutine may be damaged?

The answer under the tested Linux/amd64 conditions is yes to both. The answer to
“is arbitrary hard abort safe for application state?” remains no.

## Identity and request state

`CurrentGoroutine` returns an opaque pointer plus the current `goid`. The target
G stores one atomic 64-bit request containing the same `goid` and a two-bit
mode. A recycled G therefore cannot consume an old request. The public handle
is observational; the encoded request check is the effect fence.

Hard mode dominates a pending graceful request. A global pending counter lets
`sysmon` avoid scanning processors when no abort exists. The steady-state
monitor cost is one atomic load per monitor iteration; each G grows by one
64-bit request word.

## Abort-safe boundary

Go's existing asynchronous-preemption signal handler calls
`isAsyncSafePoint`. That gate already rejects runtime, internal-runtime,
reflection, assembly, non-Go, compiler-unsafe, stack-exhausted, locked-runtime,
and other unsuitable PCs.

The hard request is consumed at the beginning of `asyncPreempt2`, before
register state is transferred to the G. A g0 callback publishes race and trace
completion, then calls `goexit0` without executing user defers.

Graceful mode is marked ready at the same boundary and injected on the
subsequent scheduler resume. A generic scheduler handoff can never certify a
request: a newly runnable G may still need to finish internal channel, timer, or
syscall work.

## Liveness

The first safe design could leave requests pending forever when targets yielded
frequently. `sysmon` now checks a global pending count. While nonzero, it
inspects running Ps and reuses Go's existing `preemptone` machinery only for a
G whose encoded request matches its current `goid`.

This is eventually effective only when the target executes eligible Go code.
It intentionally cannot erase a goroutine that remains blocked outside that
boundary.

## Thread and signal scope

A goroutine can move between OS threads. An OS-thread signal cannot identify a
goroutine safely. Thread termination also bypasses runtime ownership.
This prototype uses the target goroutine identity, the existing safe-point
classifier, the scheduler monitor, trace and race lifecycle accounting, and the
runtime G destruction path.

## Remaining design work

- Prove or replace the best-effort semantics of `GoroutineHandle.Valid` under
  concurrent G reuse.
- Validate Windows and additional Unix architectures with native runtime tests.
- Exercise cgo return, profiler, execution-trace stress, fuzzing, and long
  randomized GC/scheduler campaigns.
- Measure no-request overhead rather than inferring it from the added load and
  field size.
- Decide whether graceful mode belongs in the same API; its semantics are
  closer to remote `Goexit` than hard abort.
- Explore capability-scoped handles so libraries cannot arbitrarily terminate
  goroutines they do not own.
