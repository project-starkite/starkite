package k8s

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/workqueue"

	"github.com/vladimirvivien/startype"

	"github.com/project-starkite/starkite/libkite"
)

// ============================================================================
// Controller — k8s.control() implementation
// ============================================================================

// queueItem holds the event info for work queue dispatch.
type queueItem struct {
	key       string                     // "namespace/name" or "name"
	eventType string                     // "ADDED", "MODIFIED", "DELETED"
	old       *unstructured.Unstructured // previous version (for MODIFIED events)
}

// controlBuiltin is the k8s.control() function that blocks like http.serve().
func (m *Module) controlBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, fmt.Errorf("k8s.control: %w", err)
	}

	// Extract callable and complex kwargs before startype parsing
	var reconcileFn, onCreateFn, onUpdateFn, onDeleteFn starlark.Callable
	var watchOwnedValue starlark.Value
	filtered := filterKwargCallable(kwargs, "reconcile", &reconcileFn)
	filtered = filterKwargCallable(filtered, "on_create", &onCreateFn)
	filtered = filterKwargCallable(filtered, "on_update", &onUpdateFn)
	filtered = filterKwargCallable(filtered, "on_delete", &onDeleteFn)
	filtered = filterKwargValue(filtered, "watch_owned", &watchOwnedValue)
	var predicateFn starlark.Callable
	filtered = filterKwargCallable(filtered, "predicate", &predicateFn)

	var p struct {
		Kind                    string `name:"kind" position:"0" required:"true"`
		Namespace               string `name:"namespace"`
		Labels                  string `name:"labels"`
		Resync                  string `name:"resync"`
		Workers                 int    `name:"workers"`
		MaxRetries              int    `name:"max_retries"`
		Backoff                 string `name:"backoff"`
		FieldSelector           string `name:"field_selector"`
		LeaderElection          bool   `name:"leader_election"`
		LeaderElectionID        string `name:"leader_election_id"`
		LeaderElectionNamespace string `name:"leader_election_namespace"`
	}
	p.Workers = 1
	p.MaxRetries = 5
	p.Backoff = "5s"
	if err := startype.Args(args, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.control: %w", err)
	}

	// Validate: at least one handler
	if reconcileFn == nil && onCreateFn == nil && onUpdateFn == nil && onDeleteFn == nil {
		return nil, fmt.Errorf("k8s.control: at least one handler required (reconcile, on_create, on_update, or on_delete)")
	}

	// Resolve GVR
	gvr, namespaced, err := client.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.control: %w", err)
	}

	// Parse durations
	var resyncInterval time.Duration
	if p.Resync != "" {
		resyncInterval, err = time.ParseDuration(p.Resync)
		if err != nil {
			return nil, fmt.Errorf("k8s.control: invalid resync %q: %w", p.Resync, err)
		}
	}
	backoff, err := time.ParseDuration(p.Backoff)
	if err != nil {
		return nil, fmt.Errorf("k8s.control: invalid backoff %q: %w", p.Backoff, err)
	}

	ns := p.Namespace
	if ns == "" && namespaced {
		ns = client.namespace
	}

	// Parse watch_owned list
	var watchOwned []string
	if watchOwnedValue != nil {
		if list, ok := watchOwnedValue.(*starlark.List); ok {
			for i := 0; i < list.Len(); i++ {
				if s, ok := starlark.AsString(list.Index(i)); ok {
					watchOwned = append(watchOwned, s)
				}
			}
		}
	}

	// Leader election defaults
	leaderID := p.LeaderElectionID
	if leaderID == "" {
		leaderID = p.Kind + "-controller"
	}
	leaderNS := p.LeaderElectionNamespace
	if leaderNS == "" {
		leaderNS = ns
		if leaderNS == "" {
			leaderNS = client.namespace
		}
	}

	ctrl := &controller{
		kind:                 p.Kind,
		gvr:                  gvr,
		namespaced:           namespaced,
		namespace:            ns,
		labels:               p.Labels,
		resync:               resyncInterval,
		workers:              p.Workers,
		maxRetries:           p.MaxRetries,
		backoff:              backoff,
		reconcileFn:          reconcileFn,
		onCreateFn:           onCreateFn,
		onUpdateFn:           onUpdateFn,
		onDeleteFn:           onDeleteFn,
		client:               client,
		thread:               thread,
		cache:                make(map[string]*unstructured.Unstructured),
		watchOwned:           watchOwned,
		predicateFn:          predicateFn,
		fieldSelector:        p.FieldSelector,
		enableLeaderElection: p.LeaderElection,
		leaderElectionID:     leaderID,
		leaderElectionNS:     leaderNS,
	}

	return ctrl.run()
}

type controller struct {
	kind          string
	gvr           schema.GroupVersionResource
	namespaced    bool
	namespace     string
	labels        string
	fieldSelector string
	resync        time.Duration
	workers       int
	maxRetries    int
	backoff       time.Duration

	reconcileFn starlark.Callable
	onCreateFn  starlark.Callable
	onUpdateFn  starlark.Callable
	onDeleteFn  starlark.Callable
	predicateFn starlark.Callable // predicate: fn(event, obj) -> bool
	watchOwned  []string          // owned resource kinds to watch (e.g., ["deployments", "services"])

	enableLeaderElection bool
	leaderElectionID     string
	leaderElectionNS     string

	client  *K8sClient
	thread  *starlark.Thread
	queue   workqueue.RateLimitingInterface
	cache   map[string]*unstructured.Unstructured
	cacheMu sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// run starts the controller and blocks until stopped.
func (c *controller) run() (starlark.Value, error) {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	defer c.cancel()

	c.queue = workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(c.backoff, 5*time.Minute))
	defer c.queue.ShutDown()

	// Install signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		c.cancel()
	}()

	// Start watch goroutine with reconnect
	var wg sync.WaitGroup
	wg.Go(func() {
		c.watchLoop()
	})

	// Start resync goroutine if configured
	if c.resync > 0 {
		wg.Go(func() {
			c.resyncLoop()
		})
	}

	// Start owned resource watch goroutines
	for _, ownedKind := range c.watchOwned {
		kind := ownedKind
		wg.Go(func() {
			c.watchOwnedLoop(kind)
		})
	}

	// Start worker goroutines (leader-only if leader election enabled)
	if c.enableLeaderElection {
		c.runWithLeaderElection(&wg)
	} else {
		c.startWorkers(&wg)
	}

	// Block until context cancelled
	<-c.ctx.Done()
	c.queue.ShutDown()
	wg.Wait()

	return starlark.None, nil
}

// startWorkers launches worker goroutines that process the queue.
func (c *controller) startWorkers(wg *sync.WaitGroup) {
	for i := 0; i < c.workers; i++ {
		wg.Go(func() {
			c.workerLoop()
		})
	}
}

// runWithLeaderElection wraps worker startup in leader election.
// Watches run always (warm cache on standbys); workers run only when leader.
func (c *controller) runWithLeaderElection(wg *sync.WaitGroup) {
	clientset, err := kubernetes.NewForConfig(c.client.restCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "k8s.control: leader election: failed to create clientset: %v\n", err)
		c.cancel()
		return
	}

	id, err := os.Hostname()
	if err != nil {
		id = fmt.Sprintf("controller-%d", os.Getpid())
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      c.leaderElectionID,
			Namespace: c.leaderElectionNS,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	wg.Go(func() {
		leaderelection.RunOrDie(c.ctx, leaderelection.LeaderElectionConfig{
			Lock:            lock,
			LeaseDuration:   15 * time.Second,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			ReleaseOnCancel: true,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					fmt.Fprintf(os.Stderr, "k8s.control: started leading (%s)\n", id)
					c.startWorkers(wg)
				},
				OnStoppedLeading: func() {
					fmt.Fprintf(os.Stderr, "k8s.control: stopped leading\n")
					c.cancel()
				},
				OnNewLeader: func(identity string) {
					if identity != id {
						fmt.Fprintf(os.Stderr, "k8s.control: new leader: %s\n", identity)
					}
				},
			},
		})
	})
}

// ============================================================================
// Watch loop — reconnecting watch with event dispatch
// ============================================================================

func (c *controller) watchLoop() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		if c.ctx.Err() != nil {
			return
		}

		err := c.doWatch()
		if c.ctx.Err() != nil {
			return // shutdown
		}

		// Log error and reconnect with backoff
		if err != nil {
			fmt.Fprintf(os.Stderr, "k8s.control: watch error: %v, reconnecting in %v\n", err, backoff)
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (c *controller) doWatch() error {
	opts := metav1.ListOptions{}
	if c.labels != "" {
		opts.LabelSelector = c.labels
	}
	if c.fieldSelector != "" {
		opts.FieldSelector = c.fieldSelector
	}

	var watcher watch.Interface
	var err error
	if c.namespaced && c.namespace != "" {
		watcher, err = c.client.dynClient.Resource(c.gvr).Namespace(c.namespace).Watch(c.ctx, opts)
	} else {
		watcher, err = c.client.dynClient.Resource(c.gvr).Watch(c.ctx, opts)
	}
	if err != nil {
		return fmt.Errorf("watch %s: %w", c.kind, err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			obj, ok := event.Object.(*unstructuredObj)
			if !ok {
				continue
			}

			// Apply predicate before enqueuing
			if c.predicateFn != nil {
				if !c.applyPredicate(string(event.Type), obj) {
					continue
				}
			}

			key := keyForObject(obj)

			// Save previous version before updating cache (for on_update old/new)
			c.cacheMu.Lock()
			old := c.cache[key]
			c.cache[key] = obj.DeepCopy()
			c.cacheMu.Unlock()

			// Enqueue with previous version
			c.queue.Add(queueItem{
				key:       key,
				eventType: string(event.Type),
				old:       old,
			})
		}
	}
}

// ============================================================================
// Resync loop — periodic re-list and re-enqueue
// ============================================================================

func (c *controller) resyncLoop() {
	ticker := time.NewTicker(c.resync)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.doResync()
		}
	}
}

func (c *controller) doResync() {
	opts := metav1.ListOptions{}
	if c.labels != "" {
		opts.LabelSelector = c.labels
	}
	if c.fieldSelector != "" {
		opts.FieldSelector = c.fieldSelector
	}

	var list *unstructured.UnstructuredList
	var err error
	if c.namespaced && c.namespace != "" {
		list, err = c.client.dynClient.Resource(c.gvr).Namespace(c.namespace).List(c.ctx, opts)
	} else {
		list, err = c.client.dynClient.Resource(c.gvr).List(c.ctx, opts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "k8s.control: resync list error: %v\n", err)
		return
	}

	for i := range list.Items {
		obj := &list.Items[i]

		// Apply predicate before enqueuing
		if c.predicateFn != nil {
			if !c.applyPredicate(string(watch.Added), obj) {
				continue
			}
		}

		key := keyForObject(obj)

		c.cacheMu.Lock()
		c.cache[key] = obj.DeepCopy()
		c.cacheMu.Unlock()

		c.queue.Add(queueItem{
			key:       key,
			eventType: string(watch.Added),
		})
	}
}

// ============================================================================
// Owned resource watch loop — watch children, enqueue parents
// ============================================================================

func (c *controller) watchOwnedLoop(ownedKind string) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		if c.ctx.Err() != nil {
			return
		}

		err := c.doWatchOwned(ownedKind)
		if c.ctx.Err() != nil {
			return
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "k8s.control: owned watch %s error: %v, reconnecting in %v\n", ownedKind, err, backoff)
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (c *controller) doWatchOwned(ownedKind string) error {
	gvr, namespaced, err := c.client.resolver.Resolve(ownedKind)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", ownedKind, err)
	}

	opts := metav1.ListOptions{}

	var watcher watch.Interface
	if namespaced && c.namespace != "" {
		watcher, err = c.client.dynClient.Resource(gvr).Namespace(c.namespace).Watch(c.ctx, opts)
	} else {
		watcher, err = c.client.dynClient.Resource(gvr).Watch(c.ctx, opts)
	}
	if err != nil {
		return fmt.Errorf("watch %s: %w", ownedKind, err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			obj, ok := event.Object.(*unstructuredObj)
			if !ok {
				continue
			}

			// Look up ownerReferences — enqueue parent if it matches the controller's primary kind
			for _, ref := range obj.GetOwnerReferences() {
				if strings.EqualFold(ref.Kind, c.kind) {
					parentKey := obj.GetNamespace() + "/" + ref.Name
					if obj.GetNamespace() == "" {
						parentKey = ref.Name
					}
					c.queue.Add(queueItem{
						key:       parentKey,
						eventType: string(watch.Modified),
					})
				}
			}
		}
	}
}

// ============================================================================
// Worker loop — dequeue and dispatch to handlers
// ============================================================================

func (c *controller) workerLoop() {
	for {
		if c.ctx.Err() != nil {
			return
		}
		if !c.processNextItem() {
			return
		}
	}
}

func (c *controller) processNextItem() bool {
	raw, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(raw)

	item, ok := raw.(queueItem)
	if !ok {
		c.queue.Forget(raw)
		return true
	}

	err := c.dispatch(item)
	if err != nil {
		if c.queue.NumRequeues(raw) < c.maxRetries {
			c.queue.AddRateLimited(raw)
		} else {
			fmt.Fprintf(os.Stderr, "k8s.control: dropping %s after %d retries: %v\n", item.key, c.maxRetries, err)
			c.queue.Forget(raw)
		}
		return true
	}

	c.queue.Forget(raw)
	return true
}

func (c *controller) dispatch(item queueItem) error {
	// Get object from cache
	c.cacheMu.RLock()
	obj, exists := c.cache[item.key]
	c.cacheMu.RUnlock()

	// For non-delete events, re-fetch if not in cache
	if !exists && item.eventType != string(watch.Deleted) {
		ns, name := splitKey(item.key)
		fetched, err := c.client.dynClient.Resource(c.gvr).Namespace(ns).Get(c.ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil // object gone, skip
		}
		obj = fetched
	}

	if obj == nil {
		return nil // nothing to reconcile
	}

	attrDict := unstructuredToAttrDict(obj)

	// Create a child thread for the handler call
	childThread := &starlark.Thread{Name: "controller-worker"}
	if c.thread != nil {
		childThread.Print = c.thread.Print
	}
	// Copy permissions from parent thread
	if perms := libkite.GetPermissions(c.thread); perms != nil {
		libkite.SetPermissions(childThread, perms)
	}

	switch item.eventType {
	case string(watch.Added):
		if c.onCreateFn != nil {
			_, err := starlark.Call(childThread, c.onCreateFn, starlark.Tuple{attrDict}, nil)
			return err
		}
		if c.reconcileFn != nil {
			_, err := starlark.Call(childThread, c.reconcileFn, starlark.Tuple{starlark.String("ADDED"), attrDict}, nil)
			return err
		}

	case string(watch.Modified):
		if c.onUpdateFn != nil {
			oldDict := attrDict // fallback if no previous version
			if item.old != nil {
				oldDict = unstructuredToAttrDict(item.old)
			}
			newDict := attrDict
			_, err := starlark.Call(childThread, c.onUpdateFn, starlark.Tuple{oldDict, newDict}, nil)
			return err
		}
		if c.reconcileFn != nil {
			_, err := starlark.Call(childThread, c.reconcileFn, starlark.Tuple{starlark.String("MODIFIED"), attrDict}, nil)
			return err
		}

	case string(watch.Deleted):
		// Remove from cache after reading
		c.cacheMu.Lock()
		delete(c.cache, item.key)
		c.cacheMu.Unlock()

		if c.onDeleteFn != nil {
			_, err := starlark.Call(childThread, c.onDeleteFn, starlark.Tuple{attrDict}, nil)
			return err
		}
		if c.reconcileFn != nil {
			_, err := starlark.Call(childThread, c.reconcileFn, starlark.Tuple{starlark.String("DELETED"), attrDict}, nil)
			return err
		}
	}

	return nil
}

// ============================================================================
// Key helpers
// ============================================================================

func keyForObject(obj *unstructured.Unstructured) string {
	ns := obj.GetNamespace()
	name := obj.GetName()
	if ns != "" {
		return ns + "/" + name
	}
	return name
}

func splitKey(key string) (namespace, name string) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

// applyPredicate calls the predicate function and returns true if the event should be enqueued.
func (c *controller) applyPredicate(eventType string, obj *unstructuredObj) bool {
	filterThread := &starlark.Thread{Name: "controller-predicate"}
	if c.thread != nil {
		filterThread.Print = c.thread.Print
	}
	attrDict := unstructuredToAttrDict(obj)
	result, err := starlark.Call(filterThread, c.predicateFn, starlark.Tuple{
		starlark.String(eventType),
		attrDict,
	}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "k8s.control: predicate error: %v\n", err)
		return false
	}
	if b, ok := result.(starlark.Bool); ok {
		return bool(b)
	}
	return true
}
