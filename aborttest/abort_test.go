package aborttest

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testTimeout = 2 * time.Second

func startAbortable(t *testing.T, body func()) runtime.GoroutineHandle {
	t.Helper()
	ready := make(chan runtime.GoroutineHandle, 1)
	go func() {
		ready <- runtime.CurrentGoroutine()
		body()
	}()
	select {
	case handle := <-ready:
		if !handle.Valid() {
			t.Fatal("new goroutine handle is not valid")
		}
		return handle
	case <-time.After(testTimeout):
		t.Fatal("goroutine did not publish its handle")
		return runtime.GoroutineHandle{}
	}
}

func waitStopped(t *testing.T, handle runtime.GoroutineHandle) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for handle.Valid() && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if handle.Valid() {
		t.Fatal("goroutine remained valid after abort deadline")
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(testTimeout):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func TestHardAbortTightLoop(t *testing.T) {
	started := make(chan struct{})
	var iterations atomic.Uint64
	handle := startAbortable(t, func() {
		close(started)
		for {
			iterations.Add(1)
		}
	})
	waitSignal(t, started, "tight loop")
	if !handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("hard abort was not accepted")
	}
	waitStopped(t, handle)
	stoppedAt := iterations.Load()
	time.Sleep(10 * time.Millisecond)
	if got := iterations.Load(); got != stoppedAt {
		t.Fatalf("counter advanced after hard abort: %d -> %d", stoppedAt, got)
	}
}

func TestGracefulAbortRunsDefers(t *testing.T) {
	started := make(chan struct{})
	deferred := make(chan struct{})
	handle := startAbortable(t, func() {
		defer close(deferred)
		close(started)
		for {
		}
	})
	waitSignal(t, started, "graceful loop")
	if !handle.Abort(runtime.GoroutineAbortGraceful) {
		t.Fatal("graceful abort was not accepted")
	}
	waitStopped(t, handle)
	waitSignal(t, deferred, "graceful defer")
}

func TestHardAbortSkipsDefersAndRecover(t *testing.T) {
	started := make(chan struct{})
	var deferred atomic.Bool
	var recovered atomic.Bool
	handle := startAbortable(t, func() {
		defer func() {
			deferred.Store(true)
			if recover() != nil {
				recovered.Store(true)
			}
		}()
		close(started)
		for {
		}
	})
	waitSignal(t, started, "hard-abort loop")
	if !handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("hard abort was not accepted")
	}
	waitStopped(t, handle)
	if deferred.Load() || recovered.Load() {
		t.Fatalf("hard abort ran user unwind: deferred=%v recovered=%v", deferred.Load(), recovered.Load())
	}
}

func TestHardAbortCanStrandUserMutex(t *testing.T) {
	var mutex sync.Mutex
	locked := make(chan struct{})
	handle := startAbortable(t, func() {
		mutex.Lock()
		close(locked)
		for {
		}
	})
	waitSignal(t, locked, "mutex acquisition")
	if !handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("hard abort was not accepted")
	}
	waitStopped(t, handle)
	if mutex.TryLock() {
		mutex.Unlock()
		t.Fatal("hard abort unexpectedly released a user mutex")
	}
}

func TestBlockedGoroutineWaitsForRunnableBoundary(t *testing.T) {
	gate := make(chan struct{})
	waiting := make(chan struct{})
	handle := startAbortable(t, func() {
		close(waiting)
		<-gate
	})
	waitSignal(t, waiting, "channel wait")
	time.Sleep(10 * time.Millisecond)
	if !handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("hard abort request was not accepted")
	}
	time.Sleep(20 * time.Millisecond)
	if !handle.Valid() {
		t.Fatal("blocked goroutine was destroyed before becoming runnable")
	}
	close(gate)
	waitStopped(t, handle)
}

func TestHardAbortEscalatesLoopingGracefulDefer(t *testing.T) {
	started := make(chan struct{})
	insideDefer := make(chan struct{})
	var once sync.Once
	handle := startAbortable(t, func() {
		defer func() {
			once.Do(func() { close(insideDefer) })
			for {
			}
		}()
		close(started)
		for {
		}
	})
	waitSignal(t, started, "escalation loop")
	if !handle.Abort(runtime.GoroutineAbortGraceful) {
		t.Fatal("graceful abort was not accepted")
	}
	waitSignal(t, insideDefer, "looping defer")
	if !handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("hard escalation was not accepted")
	}
	waitStopped(t, handle)
}

func TestStaleAndSelfHandlesCannotAbort(t *testing.T) {
	self := runtime.CurrentGoroutine()
	if self.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("self abort should be rejected")
	}
	if self.Abort(runtime.GoroutineAbortMode(255)) {
		t.Fatal("unknown abort mode should be rejected")
	}

	done := make(chan struct{})
	release := make(chan struct{})
	handle := startAbortable(t, func() {
		close(done)
		<-release
	})
	waitSignal(t, done, "ordinary completion")
	close(release)
	waitStopped(t, handle)
	if handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("stale handle aborted a recycled or dead goroutine")
	}
}

func TestHardAbortChurnWithGC(t *testing.T) {
	const cycles = 1000
	for cycle := 0; cycle < cycles; cycle++ {
		started := make(chan struct{})
		handle := startAbortable(t, func() {
			close(started)
			var retained [][]byte
			for {
				retained = append(retained, make([]byte, 256))
				if len(retained) == 64 {
					retained = retained[:0]
				}
			}
		})
		waitSignal(t, started, "allocation loop")
		if !handle.Abort(runtime.GoroutineAbortHard) {
			t.Fatalf("cycle %d: hard abort was not accepted", cycle)
		}
		waitStopped(t, handle)
		if cycle%100 == 0 {
			runtime.GC()
		}
	}
}

func TestConcurrentHardAbortCallers(t *testing.T) {
	started := make(chan struct{})
	handle := startAbortable(t, func() {
		close(started)
		for {
		}
	})
	waitSignal(t, started, "concurrent-abort loop")

	const callers = 64
	var accepted atomic.Uint32
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			if handle.Abort(runtime.GoroutineAbortHard) {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() == 0 {
		t.Fatal("no concurrent hard abort request was accepted")
	}
	waitStopped(t, handle)
}

func TestStaleHandlesDoNotTargetReusedGoroutines(t *testing.T) {
	const cycles = 256
	stale := make([]runtime.GoroutineHandle, 0, cycles)
	for range cycles {
		release := make(chan struct{})
		handle := startAbortable(t, func() { <-release })
		stale = append(stale, handle)
		close(release)
		waitStopped(t, handle)
	}

	started := make(chan struct{})
	live := startAbortable(t, func() {
		close(started)
		for {
		}
	})
	waitSignal(t, started, "reused-goroutine target")
	for index, handle := range stale {
		if handle.Abort(runtime.GoroutineAbortHard) {
			t.Fatalf("stale handle %d attached an abort request", index)
		}
	}
	if !live.Valid() {
		t.Fatal("stale handle traffic terminated the live goroutine")
	}
	if !live.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("live cleanup abort was not accepted")
	}
	waitStopped(t, live)
}

func TestHardAbortReleasesLockedOSThread(t *testing.T) {
	started := make(chan struct{})
	handle := startAbortable(t, func() {
		runtime.LockOSThread()
		close(started)
		for {
		}
	})
	waitSignal(t, started, "locked OS thread")
	if !handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("hard abort of locked goroutine was not accepted")
	}
	waitStopped(t, handle)

	completed := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		runtime.UnlockOSThread()
		close(completed)
	}()
	waitSignal(t, completed, "post-abort OS-thread use")
}

func TestHardAbortWaitsForSyscallReturn(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()

	entered := make(chan struct{})
	returned := make(chan struct{})
	handle := startAbortable(t, func() {
		close(entered)
		buffer := make([]byte, 1)
		_, _ = reader.Read(buffer)
		close(returned)
		for {
		}
	})
	waitSignal(t, entered, "pipe read")
	time.Sleep(10 * time.Millisecond)
	if !handle.Abort(runtime.GoroutineAbortHard) {
		t.Fatal("hard abort during syscall was not accepted")
	}
	time.Sleep(20 * time.Millisecond)
	if !handle.Valid() {
		t.Fatal("goroutine was destroyed while blocked outside Go code")
	}
	if _, err := writer.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, returned, "pipe read return")
	waitStopped(t, handle)
}
