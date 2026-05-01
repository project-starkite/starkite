package libkite

import (
	"os"
	"os/signal"
	"syscall"

	"go.starlark.net/starlark"
)

// setupSignalHandling sets up signal handlers.
func (rt *Runtime) setupSignalHandling() {
	ch := make(chan os.Signal, 1)
	rt.signalMu.Lock()
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
		// Call the registered handler
		starlark.Call(rt.thread, handler, starlark.Tuple{starlark.String(sig.String())}, nil)
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

// runDeferred runs all deferred functions in LIFO order.
func (rt *Runtime) runDeferred() {
	rt.deferMu.Lock()
	defer rt.deferMu.Unlock()

	// Run in LIFO order
	for i := len(rt.deferredFuncs) - 1; i >= 0; i-- {
		fn := rt.deferredFuncs[i]
		starlark.Call(rt.thread, fn, nil, nil)
	}
	rt.deferredFuncs = nil
}
