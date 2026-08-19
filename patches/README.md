# Patch series

Apply these patches in this order:

1. `go1.26.4-goroutine-abort.patch`
2. `go1.26.4-abort-safe-boundary.patch`
3. `go1.26.4-abort-sysmon-retry.patch`
4. `go1.26.4-abort-scheduler-livelock.patch`
5. `go1.26.4-abort-lifecycle-accounting.patch`

The first patch is incomplete and unsafe by itself. The later patches add the
corrections required by the tested result. Each patch targets the source tree
included in Go 1.26.4.
