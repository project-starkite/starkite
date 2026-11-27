package libkite

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"go.starlark.net/starlark"
)

const teardownHookTimeout = 10 * time.Second

func normalizeSignalName(name string) string {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SIGINT", "INT", "INTERRUPT":
		return "SIGINT"
	case "SIGTERM", "TERM", "TERMINATED":
		return "SIGTERM"
	case "SIGHUP", "HUP", "HANGUP":
		return "SIGHUP"
	default:
		return strings.ToUpper(strings.TrimSpace(name))
	}
}

func signalCanonicalName(sig os.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGHUP:
		return "SIGHUP"
	}
	if sig == nil {
		return ""
	}
	return normalizeSignalName(sig.String())
}

func signalExitCode(sig os.Signal) int {
	switch sig {
	case syscall.SIGINT:
		return ExitInterrupt
	case syscall.SIGTERM:
		return ExitTerminate
	default:
		return 0
	}
}

// setupSignalHandling sets up signal handlers idempotently.
func (rt *Runtime) setupSignalHandling() {
	rt.signalMu.Lock()
	if rt.signalChan != nil {
		rt.signalMu.Unlock()
		return
	}
	ch := make(chan os.Signal, 4)
	rt.signalChan = ch
	rt.receivedSignal = nil
	rt.signalWG.Add(1)
	rt.signalMu.Unlock()

	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		defer rt.signalWG.Done()
		for sig := range ch {
			rt.handleSignal(sig)
		}
	}()
}

// stopSignalHandling stops signal delivery and waits for the signal watcher goroutine to drain.
func (rt *Runtime) stopSignalHandling() {
	rt.signalMu.Lock()
	ch := rt.signalChan
	rt.signalChan = nil
	rt.signalMu.Unlock()
	if ch != nil {
		signal.Stop(ch)
		close(ch)
		rt.signalWG.Wait()
	}
}

// handleSignal records the received signal and cancels runtime execution so cleanup
// runs deterministically after Starlark unwinds.
func (rt *Runtime) handleSignal(sig os.Signal) {
	rt.signalMu.Lock()
	if rt.receivedSignal == nil {
		rt.receivedSignal = sig
	}
	rt.signalMu.Unlock()

	rt.cancel()
}

func (rt *Runtime) takeReceivedSignal() os.Signal {
	rt.signalMu.Lock()
	defer rt.signalMu.Unlock()
	sig := rt.receivedSignal
	rt.receivedSignal = nil
	return sig
}

func (rt *Runtime) getSignalHandler(name string) (starlark.Callable, bool) {
	canonical := normalizeSignalName(name)
	rt.signalMu.RLock()
	defer rt.signalMu.RUnlock()
	handler, ok := rt.signalHandlers[canonical]
	return handler, ok
}

func (rt *Runtime) runSignalHandler(sig os.Signal) {
	if sig == nil {
		return
	}
	canonical := signalCanonicalName(sig)
	handler, ok := rt.getSignalHandler(canonical)
	if !ok || handler == nil {
		return
	}

	// Run on a bounded non-cancelled context so cleanup work inside the handler can finish.
	sigCtx, sigCancel := context.WithTimeout(context.Background(), teardownHookTimeout)
	defer sigCancel()

	sigThread := rt.NewThread("signal:" + canonical)
	SetThreadContext(sigThread, sigCtx)
	stopWatch := watchCtx(sigCtx, sigThread)
	defer stopWatch()

	var args starlark.Tuple
	if sf, ok := handler.(*starlark.Function); ok && sf.NumParams() == 0 && !sf.HasVarargs() {
		args = nil
	} else {
		args = starlark.Tuple{starlark.String(canonical)}
	}

	if _, err := starlark.Call(sigThread, handler, args, nil); err != nil {
		fmt.Fprintf(os.Stderr, "signal handler (%s) error: %v\n", canonical, err)
	}
}

// HasSignalHandler returns true if a handler is registered for the named signal.
func (rt *Runtime) HasSignalHandler(name string) bool {
	canonical := normalizeSignalName(name)
	rt.signalMu.RLock()
	defer rt.signalMu.RUnlock()
	_, ok := rt.signalHandlers[canonical]
	return ok
}

// RegisterSignalHandler registers a Starlark callable as a signal handler.
func (rt *Runtime) RegisterSignalHandler(name string, handler starlark.Callable) {
	canonical := normalizeSignalName(name)
	rt.signalMu.Lock()
	rt.signalHandlers[canonical] = handler
	rt.signalMu.Unlock()
}

// UnregisterSignalHandler removes a signal handler by name.
func (rt *Runtime) UnregisterSignalHandler(name string) {
	canonical := normalizeSignalName(name)
	rt.signalMu.Lock()
	delete(rt.signalHandlers, canonical)
	rt.signalMu.Unlock()
}

// runDeferred runs Starlark defer() callbacks (LIFO), then Go-side cleanups
// (e.g. open DB connections). Runs at the end of Execute and tests.
func (rt *Runtime) runDeferred() {
	rt.deferMu.Lock()
	fns := rt.deferredFuncs
	rt.deferredFuncs = nil
	rt.deferMu.Unlock()

	if len(fns) > 0 {
		deferCtx, deferCancel := context.WithTimeout(context.Background(), teardownHookTimeout)
		defer deferCancel()

		deferThread := rt.NewThread("defer")
		SetThreadContext(deferThread, deferCtx)
		stopWatch := watchCtx(deferCtx, deferThread)
		defer stopWatch()

		for _, fn := range slices.Backward(fns) {
			if _, err := starlark.Call(deferThread, fn, nil, nil); err != nil {
				fmt.Fprintf(os.Stderr, "deferred function error: %v\n", err)
			}
		}
	}
	rt.runGoCleanups()
}
