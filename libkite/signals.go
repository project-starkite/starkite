package libkite

import (
	"os"
	"os/signal"
	"slices"
	"syscall"

	"go.starlark.net/starlark"
)

// setupSignalHandling sets up signal handlers idempotently.
func (rt *Runtime) setupSignalHandling() {
	rt.signalMu.Lock()
	if rt.signalChan != nil {
		rt.signalMu.Unlock()
		return
	}
	ch := make(chan os.Signal, 1)
	rt.signalChan = ch
	rt.signalMu.Unlock()
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range ch {
			rt.handleSignal(sig)
		}
	}()
}

// stopSignalHandling stops signal handling and cleans up.
func (rt *Runtime) stopSignalHandling() {
	rt.signalMu.Lock()
	ch := rt.signalChan
	rt.signalChan = nil
	rt.signalMu.Unlock()
	if ch != nil {
		signal.Stop(ch)
		close(ch)
	}
}

// handleSignal handles a received signal.
func (rt *Runtime) handleSignal(sig os.Signal) {
	rt.signalMu.RLock()
	handler, ok := rt.signalHandlers[sig.String()]
	rt.signalMu.RUnlock()

	if ok {
		sigThread := rt.NewThread("signal:" + sig.String())
		_, _ = CallSafe(sigThread, handler, starlark.Tuple{starlark.String(sig.String())}, nil)
	}

	// Run deferred functions
	rt.runDeferred()

	// Exit with appropriate code
	switch sig {
	case syscall.SIGINT:
		os.Exit(ExitInterrupt)
	case syscall.SIGTERM:
		os.Exit(ExitTerminate)
	}
}

// HasSignalHandler returns true if a handler is registered for the named signal.
func (rt *Runtime) HasSignalHandler(name string) bool {
	rt.signalMu.RLock()
	defer rt.signalMu.RUnlock()
	_, ok := rt.signalHandlers[name]
	return ok
}

// RegisterSignalHandler registers a Starlark callable as a signal handler.
func (rt *Runtime) RegisterSignalHandler(name string, handler starlark.Callable) {
	rt.signalMu.Lock()
	rt.signalHandlers[name] = handler
	rt.signalMu.Unlock()
}

// UnregisterSignalHandler removes a signal handler by name.
func (rt *Runtime) UnregisterSignalHandler(name string) {
	rt.signalMu.Lock()
	delete(rt.signalHandlers, name)
	rt.signalMu.Unlock()
}

// runDeferred runs Starlark defer() callbacks (LIFO), then Go-side cleanups
// (e.g. open DB connections). Runs at the end of Execute and on signal.
func (rt *Runtime) runDeferred() {
	rt.deferMu.Lock()
	fns := rt.deferredFuncs
	rt.deferredFuncs = nil
	rt.deferMu.Unlock()

	// Run in LIFO order
	if len(fns) > 0 {
		deferThread := rt.NewThread("defer")
		for _, fn := range slices.Backward(fns) {
			_, _ = CallSafe(deferThread, fn, nil, nil)
		}
	}
	rt.runGoCleanups()
}
