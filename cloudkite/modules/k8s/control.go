package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.starlark.net/starlark"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
	var reconcileFn, onCreateFn, onUpdateFn, onDeleteFn, finalizeFn starlark.Callable
	var watchOwnedValue starlark.Value
	var watchRelatedValue starlark.Value
	filtered := filterKwargCallable(kwargs, "reconcile", &reconcileFn)
	filtered = filterKwargCallable(filtered, "on_create", &onCreateFn)
	filtered = filterKwargCallable(filtered, "on_update", &onUpdateFn)
	filtered = filterKwargCallable(filtered, "on_delete", &onDeleteFn)
	filtered = filterKwargCallable(filtered, "finalize", &finalizeFn)
	filtered = filterKwargValue(filtered, "watch_owned", &watchOwnedValue)
	filtered = filterKwargValue(filtered, "watch_related", &watchRelatedValue)
	var predicateFn starlark.Callable
	filtered = filterKwargCallable(filtered, "predicate", &predicateFn)

	var p struct {
		Kind                    string `name:"kind" position:"0" required:"true"`
		Namespace               string `name:"namespace"`
		Labels                  string `name:"labels"`
		Resync                  string `name:"resync"`
		Poll                    string `name:"poll"`
		Finalizer               string `name:"finalizer"`
		HealthPort              int    `name:"health_port"`
		Workers                 int    `name:"workers"`
		MaxRetries              int    `name:"max_retries"`
		Backoff                 string `name:"backoff"`
		FieldSelector           string `name:"field_selector"`
		GenerationChanged       *bool  `name:"generation_changed"`
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

	genChanged := true
	if p.GenerationChanged != nil {
		genChanged = *p.GenerationChanged
	}

	// Validate: at least one handler
	if reconcileFn == nil && onCreateFn == nil && onUpdateFn == nil && onDeleteFn == nil && finalizeFn == nil {
		return nil, fmt.Errorf("k8s.control: at least one handler required (reconcile, finalize, on_create, on_update, or on_delete)")
	}

	// Resolve GVR
	gvr, namespaced, err := client.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.control: %w", err)
	}

	// Compute default finalizer name if finalize is configured
	finalizerName := p.Finalizer
	if finalizerName == "" && finalizeFn != nil {
		if gvr.Group != "" {
			finalizerName = fmt.Sprintf("%s.%s/finalizer", strings.ToLower(gvr.Resource), gvr.Group)
		} else {
			finalizerName = fmt.Sprintf("%s/finalizer", strings.ToLower(gvr.Resource))
		}
	}

	// Parse durations
	var resyncInterval time.Duration
	if p.Resync != "" {
		resyncInterval, err = time.ParseDuration(p.Resync)
		if err != nil {
			return nil, fmt.Errorf("k8s.control: invalid resync %q: %w", p.Resync, err)
		}
	}
	var pollInterval time.Duration
	if p.Poll != "" {
		pollInterval, err = time.ParseDuration(p.Poll)
		if err != nil {
			return nil, fmt.Errorf("k8s.control: invalid poll %q: %w", p.Poll, err)
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

	// Parse watch_related list
	var relatedWatchers []relatedWatcher
	if watchRelatedValue != nil && watchRelatedValue != starlark.None {
		var list []starlark.Value
		switch v := watchRelatedValue.(type) {
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				list = append(list, v.Index(i))
			}
		case starlark.Tuple:
			list = append(list, v...)
		default:
			return nil, fmt.Errorf("k8s.control: 'watch_related' must be a list or tuple, got %s", watchRelatedValue.Type())
		}

		for idx, item := range list {
			var rw relatedWatcher
			switch elem := item.(type) {
			case *starlark.Dict:
				kindVal, found, _ := elem.Get(starlark.String("kind"))
				if !found {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] missing 'kind'", idx)
				}
				kindStr, ok := starlark.AsString(kindVal)
				if !ok {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] 'kind' must be string", idx)
				}
				rw.kind = kindStr

				mapVal, found, _ := elem.Get(starlark.String("map_func"))
				if !found {
					mapVal, found, _ = elem.Get(starlark.String("map"))
				}
				if !found {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] missing 'map_func'", idx)
				}
				mapCallable, ok := mapVal.(starlark.Callable)
				if !ok {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] 'map_func' must be callable, got %s", idx, mapVal.Type())
				}
				rw.mapFn = mapCallable

			case *AttrDict:
				kVal, err := elem.Attr("kind")
				if err != nil || kVal == nil {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] missing 'kind'", idx)
				}
				kindStr, ok := starlark.AsString(kVal)
				if !ok {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] 'kind' must be string", idx)
				}
				rw.kind = kindStr

				mapVal, err := elem.Attr("map_func")
				if err != nil || mapVal == nil {
					mapVal, _ = elem.Attr("map")
				}
				if mapVal == nil {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] missing 'map_func'", idx)
				}
				mapCallable, ok := mapVal.(starlark.Callable)
				if !ok {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] 'map_func' must be callable", idx)
				}
				rw.mapFn = mapCallable

			case starlark.Tuple:
				if elem.Len() < 2 {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] tuple must have (kind, map_func)", idx)
				}
				kindStr, ok := starlark.AsString(elem.Index(0))
				if !ok {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] kind must be string", idx)
				}
				mapCallable, ok := elem.Index(1).(starlark.Callable)
				if !ok {
					return nil, fmt.Errorf("k8s.control: watch_related[%d] map_func must be callable", idx)
				}
				rw.kind = kindStr
				rw.mapFn = mapCallable

			default:
				return nil, fmt.Errorf("k8s.control: watch_related[%d] must be a dict or tuple, got %s", idx, item.Type())
			}

			rgvr, rnamespaced, err := client.resolver.Resolve(rw.kind)
			if err != nil {
				return nil, fmt.Errorf("k8s.control: watch_related[%d] resolve %q: %w", idx, rw.kind, err)
			}
			rw.gvr = rgvr
			rw.namespaced = rnamespaced
			relatedWatchers = append(relatedWatchers, rw)
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
		poll:                 pollInterval,
		workers:              p.Workers,
		maxRetries:           p.MaxRetries,
		backoff:              backoff,
		generationChanged:    genChanged,
		reconcileFn:          reconcileFn,
		onCreateFn:           onCreateFn,
		onUpdateFn:           onUpdateFn,
		onDeleteFn:           onDeleteFn,
		finalizeFn:           finalizeFn,
		finalizerName:        finalizerName,
		healthPort:           p.HealthPort,
		watchRelated:         relatedWatchers,
		client:               client,
		thread:               thread,
		cache:                make(map[string]*unstructured.Unstructured),
		watchOwned:           watchOwned,
		predicateFn:          predicateFn,
		fieldSelector:        p.FieldSelector,
		enableLeaderElection: p.LeaderElection,
		leaderElectionID:     leaderID,
		leaderElectionNS:     leaderNS,
		echoKeys:             make(map[string]time.Time),
		watchedGVRs:          make(map[schema.GroupVersionResource]context.CancelFunc),
	}

	return ctrl.run()
}

type relatedWatcher struct {
	kind       string
	gvr        schema.GroupVersionResource
	namespaced bool
	mapFn      starlark.Callable
}

type controller struct {
	kind          string
	gvr           schema.GroupVersionResource
	namespaced    bool
	namespace     string
	labels        string
	fieldSelector string
	resync        time.Duration
	poll          time.Duration
	workers       int
	maxRetries    int
	backoff       time.Duration

	generationChanged bool

	reconcileFn   starlark.Callable
	onCreateFn    starlark.Callable
	onUpdateFn    starlark.Callable
	onDeleteFn    starlark.Callable
	finalizeFn    starlark.Callable
	finalizerName string
	predicateFn   starlark.Callable // predicate: fn(event, obj) -> bool
	watchOwned    []string          // owned resource kinds to watch (e.g., ["deployments", "services"])

	healthPort   int
	leading      atomic.Bool
	watchRelated []relatedWatcher

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

	echoMu   sync.RWMutex
	echoKeys map[string]time.Time

	autoWatchMu sync.Mutex
	watchedGVRs map[schema.GroupVersionResource]context.CancelFunc
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
		if gvr, _, err := c.client.resolver.Resolve(ownedKind); err == nil {
			c.AutoWatch(gvr)
		} else {
			kind := ownedKind
			wg.Go(func() {
				c.watchOwnedLoop(kind)
			})
		}
	}

	// Start related resource watch goroutines
	for _, rw := range c.watchRelated {
		watcher := rw
		wg.Go(func() {
			c.watchRelatedLoop(watcher)
		})
	}

	// Start health server if configured
	if c.healthPort > 0 {
		wg.Go(func() {
			c.runHealthServer()
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
	c.leading.Store(true)
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
					c.leading.Store(true)
					fmt.Fprintf(os.Stderr, "k8s.control: started leading (%s)\n", id)
					c.startWorkers(wg)
				},
				OnStoppedLeading: func() {
					c.leading.Store(false)
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

			// Suppress self-echoed events from status updates performed by this controller
			uid := string(obj.GetUID())
			rv := obj.GetResourceVersion()
			if uid != "" && rv != "" && c.isSelfEcho(uid, rv) {
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

			// Inherent generation filter: skip enqueuing when generation hasn't changed on MODIFIED events
			if c.generationChanged && event.Type == watch.Modified {
				if old != nil && obj.GetGeneration() != 0 && old.GetGeneration() == obj.GetGeneration() {
					continue
				}
			}

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
				if c.matchesOwnerRef(ref) {
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

// AutoWatch registers a child GVR to be watched dynamically.
// If an event occurs on a resource of this GVR with an ownerReference
// pointing to the controller's kind, the parent custom resource is enqueued.
func (c *controller) AutoWatch(gvr schema.GroupVersionResource) {
	c.autoWatchMu.Lock()
	defer c.autoWatchMu.Unlock()

	if _, exists := c.watchedGVRs[gvr]; exists || gvr == c.gvr {
		return
	}

	watchCtx, cancel := context.WithCancel(c.ctx)
	c.watchedGVRs[gvr] = cancel

	go c.runDynamicChildWatch(watchCtx, gvr)
}

// RecordSelfEcho records the UID and resourceVersion from a status update performed by this controller.
func (c *controller) RecordSelfEcho(uid, resourceVersion string) {
	c.echoMu.Lock()
	defer c.echoMu.Unlock()

	now := time.Now()
	for k, t := range c.echoKeys {
		if now.Sub(t) > time.Minute {
			delete(c.echoKeys, k)
		}
	}
	c.echoKeys[uid+":"+resourceVersion] = now
}

// isSelfEcho returns true if the incoming event matches a recorded self-echo, and consumes the entry.
func (c *controller) isSelfEcho(uid, resourceVersion string) bool {
	c.echoMu.Lock()
	defer c.echoMu.Unlock()

	key := uid + ":" + resourceVersion
	if _, exists := c.echoKeys[key]; exists {
		delete(c.echoKeys, key)
		return true
	}
	return false
}

func (c *controller) runDynamicChildWatch(ctx context.Context, gvr schema.GroupVersionResource) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		err := c.doWatchDynamicChild(ctx, gvr)
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "k8s.control: dynamic child watch %v error: %v, reconnecting in %v\n", gvr, err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (c *controller) doWatchDynamicChild(ctx context.Context, gvr schema.GroupVersionResource) error {
	opts := metav1.ListOptions{}

	var watcher watch.Interface
	var err error
	if c.namespaced && c.namespace != "" {
		watcher, err = c.client.dynClient.Resource(gvr).Namespace(c.namespace).Watch(ctx, opts)
	} else {
		watcher, err = c.client.dynClient.Resource(gvr).Watch(ctx, opts)
	}
	if err != nil {
		return fmt.Errorf("watch child %v: %w", gvr, err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("child watch channel closed")
			}

			obj, ok := event.Object.(*unstructuredObj)
			if !ok {
				continue
			}

			// Check ownerReferences — enqueue parent if it matches controller kind
			for _, ref := range obj.GetOwnerReferences() {
				if c.matchesOwnerRef(ref) {
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

func (c *controller) matchesOwnerRef(ref metav1.OwnerReference) bool {
	if strings.EqualFold(ref.Kind, c.kind) ||
		strings.EqualFold(ref.Kind+"s", c.kind) ||
		strings.EqualFold(ref.Kind, c.kind+"s") ||
		strings.EqualFold(ref.Kind, c.gvr.Resource) ||
		strings.EqualFold(ref.Kind+"s", c.gvr.Resource) {
		return true
	}
	if c.client != nil && c.client.resolver != nil {
		if gvr, _, err := c.client.resolver.Resolve(ref.Kind); err == nil {
			return gvr == c.gvr
		}
	}
	return false
}

func (c *controller) watchRelatedLoop(r relatedWatcher) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		if c.ctx.Err() != nil {
			return
		}

		err := c.doWatchRelated(r)
		if c.ctx.Err() != nil {
			return
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "k8s.control: related watch %s error: %v, reconnecting in %v\n", r.kind, err, backoff)
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (c *controller) doWatchRelated(r relatedWatcher) error {
	opts := metav1.ListOptions{}

	var watcher watch.Interface
	var err error
	if r.namespaced && c.namespace != "" {
		watcher, err = c.client.dynClient.Resource(r.gvr).Namespace(c.namespace).Watch(c.ctx, opts)
	} else {
		watcher, err = c.client.dynClient.Resource(r.gvr).Watch(c.ctx, opts)
	}
	if err != nil {
		return fmt.Errorf("watch related %s: %w", r.kind, err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("related watch channel closed")
			}

			obj, ok := event.Object.(*unstructuredObj)
			if !ok {
				continue
			}

			childThread := &starlark.Thread{Name: "related-mapper"}
			if c.thread != nil {
				childThread.Print = c.thread.Print
			}
			if perms := libkite.GetPermissions(c.thread); perms != nil {
				libkite.SetPermissions(childThread, perms)
			}
			childThread.SetLocal(ActiveControllerKey, c)

			attrDict := unstructuredToAttrDict(obj)
			res, err := starlark.Call(childThread, r.mapFn, starlark.Tuple{attrDict}, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "k8s.control: map_func error for %s on %s/%s: %v\n", r.kind, obj.GetNamespace(), obj.GetName(), err)
				continue
			}

			c.enqueueMappedKeys(res)
		}
	}
}

func (c *controller) enqueueMappedKeys(res starlark.Value) {
	if res == nil || res == starlark.None {
		return
	}

	enqueueOne := func(v starlark.Value) {
		switch item := v.(type) {
		case starlark.String:
			key := string(item)
			if key != "" {
				if !strings.Contains(key, "/") && c.namespaced && c.namespace != "" {
					key = c.namespace + "/" + key
				}
				c.queue.Add(queueItem{
					key:       key,
					eventType: string(watch.Modified),
				})
			}
		case *starlark.Dict:
			var name, ns string
			if nVal, found, _ := item.Get(starlark.String("name")); found {
				if s, ok := starlark.AsString(nVal); ok {
					name = s
				}
			}
			if nsVal, found, _ := item.Get(starlark.String("namespace")); found {
				if s, ok := starlark.AsString(nsVal); ok {
					ns = s
				}
			}
			if name != "" {
				key := name
				if ns != "" {
					key = ns + "/" + name
				} else if c.namespaced && c.namespace != "" {
					key = c.namespace + "/" + name
				}
				c.queue.Add(queueItem{
					key:       key,
					eventType: string(watch.Modified),
				})
			}
		case *AttrDict:
			m := item.ToMap()
			name, _ := m["name"].(string)
			ns, _ := m["namespace"].(string)
			if name != "" {
				key := name
				if ns != "" {
					key = ns + "/" + name
				} else if c.namespaced && c.namespace != "" {
					key = c.namespace + "/" + name
				}
				c.queue.Add(queueItem{
					key:       key,
					eventType: string(watch.Modified),
				})
			}
		}
	}

	switch v := res.(type) {
	case *starlark.List:
		for i := 0; i < v.Len(); i++ {
			enqueueOne(v.Index(i))
		}
	case starlark.Tuple:
		for _, elem := range v {
			enqueueOne(elem)
		}
	default:
		enqueueOne(v)
	}
}

func (c *controller) healthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if c.ctx != nil && c.ctx.Err() != nil {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if c.ctx != nil && c.ctx.Err() != nil {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		if c.enableLeaderElection && !c.leading.Load() {
			http.Error(w, "standby (not leader)", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func (c *controller) runHealthServer() {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", c.healthPort),
		Handler: c.healthHandler(),
	}

	go func() {
		<-c.ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "k8s.control: health server listening on :%d\n", c.healthPort)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "k8s.control: health server error: %v\n", err)
	}
}

func (c *controller) emitEvent(obj *unstructured.Unstructured, eventType, reason, message string) {
	if c.client == nil {
		return
	}
	cs, err := c.client.getClientset()
	if err != nil || cs == nil {
		return
	}

	ns := obj.GetNamespace()
	if ns == "" {
		ns = c.namespace
	}
	if ns == "" {
		ns = "default"
	}

	t := metav1.Now()
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%x", obj.GetName(), t.UnixNano()),
			Namespace: ns,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			UID:        obj.GetUID(),
		},
		Reason:         reason,
		Message:        message,
		Type:           eventType,
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Source: corev1.EventSource{
			Component: "starkite-controller",
		},
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	_, _ = cs.CoreV1().Events(ns).Create(ctx, ev, metav1.CreateOptions{})
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
		fmt.Fprintf(os.Stderr, "k8s.control: dispatch error on %s: %v\n", item.key, err)
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

	if !exists && item.eventType != string(watch.Deleted) {
		ns, name := splitKey(item.key)
		var fetched *unstructured.Unstructured
		var err error
		if c.namespaced && ns != "" {
			fetched, err = c.client.dynClient.Resource(c.gvr).Namespace(ns).Get(c.ctx, name, metav1.GetOptions{})
		} else {
			fetched, err = c.client.dynClient.Resource(c.gvr).Get(c.ctx, name, metav1.GetOptions{})
		}
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
	childThread.SetLocal(ActiveControllerKey, c)

	isDeleting := !obj.GetDeletionTimestamp().IsZero()

	// Declarative teardown hook (finalize)
	if c.finalizeFn != nil {
		if isDeleting {
			if hasFinalizer(obj, c.finalizerName) {
				c.emitEvent(obj, corev1.EventTypeNormal, "Finalizing", "Executing teardown hook")
				if err := c.callFinalize(childThread, attrDict); err != nil {
					fmt.Fprintf(os.Stderr, "k8s.control: finalize error on %s: %v\n", item.key, err)
					c.updateReadyCondition(obj, false, "FinalizeError", err.Error())
					c.emitEvent(obj, corev1.EventTypeWarning, "FinalizeError", err.Error())
					return err
				}
				// Teardown succeeded cleanly. Strip finalizer to allow deletion.
				if err := c.removeFinalizer(obj); err != nil {
					return fmt.Errorf("strip finalizer %q on %s: %w", c.finalizerName, item.key, err)
				}
				c.emitEvent(obj, corev1.EventTypeNormal, "Finalized", "Teardown completed")
				return nil
			}
			return nil
		}

		// Resource is active: ensure finalizer is injected
		if !hasFinalizer(obj, c.finalizerName) {
			if err := c.addFinalizer(obj); err != nil {
				return fmt.Errorf("add finalizer %q on %s: %w", c.finalizerName, item.key, err)
			}
			attrDict = unstructuredToAttrDict(obj)
		}
	}

	var ret starlark.Value
	var err error

	switch item.eventType {
	case string(watch.Added):
		if c.onCreateFn != nil {
			ret, err = starlark.Call(childThread, c.onCreateFn, starlark.Tuple{attrDict}, nil)
		} else if c.reconcileFn != nil {
			ret, err = c.callReconcile(childThread, "ADDED", attrDict)
		}

	case string(watch.Modified):
		if c.onUpdateFn != nil {
			oldDict := attrDict // fallback if no previous version
			if item.old != nil {
				oldDict = unstructuredToAttrDict(item.old)
			}
			newDict := attrDict
			ret, err = starlark.Call(childThread, c.onUpdateFn, starlark.Tuple{oldDict, newDict}, nil)
		} else if c.reconcileFn != nil {
			ret, err = c.callReconcile(childThread, "MODIFIED", attrDict)
		}

	case string(watch.Deleted):
		// Remove from cache after reading
		c.cacheMu.Lock()
		delete(c.cache, item.key)
		c.cacheMu.Unlock()

		if c.onDeleteFn != nil {
			ret, err = starlark.Call(childThread, c.onDeleteFn, starlark.Tuple{attrDict}, nil)
		} else if c.reconcileFn != nil {
			ret, err = c.callReconcile(childThread, "DELETED", attrDict)
		}
	}

	if err != nil {
		if !isDeleting {
			c.updateReadyCondition(obj, false, "ReconcileError", err.Error())
			c.emitEvent(obj, corev1.EventTypeWarning, "ReconcileError", err.Error())
		}
		return err
	}

	// Handle functional return values for non-deleted objects
	if ret != nil && item.eventType != string(watch.Deleted) && !isDeleting {
		if list, ok := ret.(*starlark.List); ok {
			if err := c.reconcileDesiredChildren(obj, list); err != nil {
				c.updateReadyCondition(obj, false, "ReconcileError", err.Error())
				c.emitEvent(obj, corev1.EventTypeWarning, "ReconcileError", err.Error())
				return err
			}
		} else if durStr, ok := starlark.AsString(ret); ok {
			if dur, parseErr := time.ParseDuration(durStr); parseErr == nil && dur > 0 {
				c.queue.AddAfter(item, dur)
			}
		}
	}

	// Automatically infer Ready condition on successful reconciliation
	if !isDeleting && (c.reconcileFn != nil || c.onCreateFn != nil || c.onUpdateFn != nil) {
		c.updateReadyCondition(obj, true, "Reconciled", "Resource synchronized successfully")
		c.emitEvent(obj, corev1.EventTypeNormal, "Reconciled", "Resource synchronized successfully")
	}

	// Controller-level periodic polling
	if c.poll > 0 && item.eventType != string(watch.Deleted) && !isDeleting {
		if _, isDur := starlark.AsString(ret); !isDur {
			c.queue.AddAfter(item, c.poll)
		}
	}

	return nil
}

func (c *controller) callFinalize(thread *starlark.Thread, attrDict *AttrDict) error {
	if fn, ok := c.finalizeFn.(*starlark.Function); ok {
		if fn.NumParams() == 1 {
			_, err := starlark.Call(thread, c.finalizeFn, starlark.Tuple{attrDict}, nil)
			return err
		}
	}
	_, err := starlark.Call(thread, c.finalizeFn, starlark.Tuple{attrDict}, nil)
	if err != nil && strings.Contains(err.Error(), "got 1 arguments, want 2") {
		_, err = starlark.Call(thread, c.finalizeFn, starlark.Tuple{starlark.String("DELETING"), attrDict}, nil)
	}
	return err
}

func hasFinalizer(obj *unstructured.Unstructured, finalizerName string) bool {
	return slices.Contains(obj.GetFinalizers(), finalizerName)
}

func (c *controller) addFinalizer(obj *unstructured.Unstructured) error {
	finalizers := obj.GetFinalizers()
	if slices.Contains(finalizers, c.finalizerName) {
		return nil
	}
	finalizers = append(finalizers, c.finalizerName)

	patchData, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"finalizers": finalizers,
		},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	var updated *unstructured.Unstructured
	if c.namespaced && obj.GetNamespace() != "" {
		updated, err = c.client.dynClient.Resource(c.gvr).Namespace(obj.GetNamespace()).Patch(ctx, obj.GetName(), types.MergePatchType, patchData, metav1.PatchOptions{})
	} else {
		updated, err = c.client.dynClient.Resource(c.gvr).Patch(ctx, obj.GetName(), types.MergePatchType, patchData, metav1.PatchOptions{})
	}
	if err != nil {
		return err
	}
	if updated != nil {
		c.RecordSelfEcho(string(updated.GetUID()), updated.GetResourceVersion())
		c.cacheMu.Lock()
		c.cache[keyForObject(updated)] = updated.DeepCopy()
		c.cacheMu.Unlock()
		*obj = *updated
	}
	return nil
}

func (c *controller) removeFinalizer(obj *unstructured.Unstructured) error {
	finalizers := obj.GetFinalizers()
	var newFinalizers []string
	found := false
	for _, f := range finalizers {
		if f == c.finalizerName {
			found = true
			continue
		}
		newFinalizers = append(newFinalizers, f)
	}
	if !found {
		return nil
	}

	patchData, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"finalizers": newFinalizers,
		},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	var updated *unstructured.Unstructured
	if c.namespaced && obj.GetNamespace() != "" {
		updated, err = c.client.dynClient.Resource(c.gvr).Namespace(obj.GetNamespace()).Patch(ctx, obj.GetName(), types.MergePatchType, patchData, metav1.PatchOptions{})
	} else {
		updated, err = c.client.dynClient.Resource(c.gvr).Patch(ctx, obj.GetName(), types.MergePatchType, patchData, metav1.PatchOptions{})
	}
	if err != nil {
		return err
	}
	if updated != nil {
		c.RecordSelfEcho(string(updated.GetUID()), updated.GetResourceVersion())
		c.cacheMu.Lock()
		c.cache[keyForObject(updated)] = updated.DeepCopy()
		c.cacheMu.Unlock()
		*obj = *updated
	}
	return nil
}

func (c *controller) updateReadyCondition(obj *unstructured.Unstructured, ready bool, reason, message string) {
	status := "False"
	if ready {
		status = "True"
	}

	rawConds, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	lastTransition := time.Now().UTC().Format(time.RFC3339)

	// Preserve lastTransitionTime if status unchanged
	for _, raw := range rawConds {
		if cm, ok := raw.(map[string]any); ok {
			if cm["type"] == "Ready" && cm["status"] == status {
				if prevTime, ok := cm["lastTransitionTime"].(string); ok && prevTime != "" {
					lastTransition = prevTime
				}
				break
			}
		}
	}

	condMap := map[string]any{
		"type":               "Ready",
		"status":             status,
		"observedGeneration": obj.GetGeneration(),
		"lastTransitionTime": lastTransition,
		"reason":             reason,
		"message":            message,
	}

	found := false
	for i, raw := range rawConds {
		if cm, ok := raw.(map[string]any); ok {
			if cm["type"] == "Ready" {
				rawConds[i] = condMap
				found = true
				break
			}
		}
	}
	if !found {
		rawConds = append(rawConds, condMap)
	}

	_ = unstructured.SetNestedSlice(obj.Object, rawConds, "status", "conditions")

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	var updated *unstructured.Unstructured
	var err error
	if c.namespaced && obj.GetNamespace() != "" {
		updated, err = c.client.dynClient.Resource(c.gvr).Namespace(obj.GetNamespace()).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	} else {
		updated, err = c.client.dynClient.Resource(c.gvr).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	}
	if err != nil {
		return
	}
	if updated != nil {
		c.RecordSelfEcho(string(updated.GetUID()), updated.GetResourceVersion())
		c.cacheMu.Lock()
		c.cache[keyForObject(updated)] = updated.DeepCopy()
		c.cacheMu.Unlock()
	}
}

func (c *controller) callReconcile(thread *starlark.Thread, eventType string, attrDict *AttrDict) (starlark.Value, error) {
	if fn, ok := c.reconcileFn.(*starlark.Function); ok {
		if fn.NumParams() == 1 {
			return starlark.Call(thread, c.reconcileFn, starlark.Tuple{attrDict}, nil)
		}
	}
	res, err := starlark.Call(thread, c.reconcileFn, starlark.Tuple{starlark.String(eventType), attrDict}, nil)
	if err != nil && strings.Contains(err.Error(), "got 2 arguments, want 1") {
		return starlark.Call(thread, c.reconcileFn, starlark.Tuple{attrDict}, nil)
	}
	return res, err
}

func (c *controller) reconcileDesiredChildren(parentObj *unstructured.Unstructured, childrenList *starlark.List) error {
	objs, err := parseManifest(childrenList)
	if err != nil {
		return fmt.Errorf("reconcile child resources: %w", err)
	}

	parentUID := parentObj.GetUID()
	parentNs := parentObj.GetNamespace()
	if parentNs == "" && c.namespaced {
		parentNs = c.namespace
	}

	desiredKeys := make(map[string]bool)

	for _, child := range objs {
		childKind := child.GetKind()
		childGVR, namespaced, err := c.client.resolver.Resolve(childKind)
		if err != nil {
			return fmt.Errorf("resolve child kind %q: %w", childKind, err)
		}

		childNs := child.GetNamespace()
		if childNs == "" && namespaced {
			childNs = parentNs
			child.SetNamespace(childNs)
		}

		// Inject ownerReference pointing to parent
		trueVal := true
		ownerRef := metav1.OwnerReference{
			APIVersion:         parentObj.GetAPIVersion(),
			Kind:               parentObj.GetKind(),
			Name:               parentObj.GetName(),
			UID:                parentUID,
			Controller:         &trueVal,
			BlockOwnerDeletion: &trueVal,
		}
		addOrUpdateOwnerRef(child, ownerRef)

		// AutoWatch this child GVR
		c.AutoWatch(childGVR)

		// Server-Side Apply
		data, err := json.Marshal(child.Object)
		if err != nil {
			return fmt.Errorf("marshal child %s/%s: %w", childKind, child.GetName(), err)
		}

		opts := metav1.PatchOptions{
			FieldManager: "starkite",
			Force:        &trueVal,
		}

		if namespaced {
			_, err = c.client.dynClient.Resource(childGVR).Namespace(childNs).Patch(c.ctx, child.GetName(), types.ApplyPatchType, data, opts)
		} else {
			_, err = c.client.dynClient.Resource(childGVR).Patch(c.ctx, child.GetName(), types.ApplyPatchType, data, opts)
		}
		if err != nil {
			return fmt.Errorf("apply child %s/%s: %w", childKind, child.GetName(), err)
		}

		key := childGVR.String() + "/" + childNs + "/" + child.GetName()
		desiredKeys[key] = true
	}

	// Auto-prune orphaned children
	c.pruneOrphanedChildren(string(parentUID), parentNs, desiredKeys)

	return nil
}

func addOrUpdateOwnerRef(child *unstructured.Unstructured, newRef metav1.OwnerReference) {
	refs := child.GetOwnerReferences()
	found := false
	for i, ref := range refs {
		if ref.UID == newRef.UID {
			refs[i] = newRef
			found = true
			break
		}
	}
	if !found {
		refs = append(refs, newRef)
	}
	child.SetOwnerReferences(refs)
}

func (c *controller) pruneOrphanedChildren(parentUID, parentNs string, desiredKeys map[string]bool) {
	c.autoWatchMu.Lock()
	gvrs := make([]schema.GroupVersionResource, 0, len(c.watchedGVRs))
	for gvr := range c.watchedGVRs {
		gvrs = append(gvrs, gvr)
	}
	c.autoWatchMu.Unlock()

	prop := metav1.DeletePropagationBackground
	delOpts := metav1.DeleteOptions{PropagationPolicy: &prop}

	for _, gvr := range gvrs {
		var list *unstructured.UnstructuredList
		var err error
		if c.namespaced && parentNs != "" {
			list, err = c.client.dynClient.Resource(gvr).Namespace(parentNs).List(c.ctx, metav1.ListOptions{})
		} else {
			list, err = c.client.dynClient.Resource(gvr).List(c.ctx, metav1.ListOptions{})
		}
		if err != nil {
			continue
		}

		for i := range list.Items {
			item := &list.Items[i]
			isOwned := false
			for _, ref := range item.GetOwnerReferences() {
				if string(ref.UID) == parentUID {
					isOwned = true
					break
				}
			}
			if !isOwned {
				continue
			}

			key := gvr.String() + "/" + item.GetNamespace() + "/" + item.GetName()
			if !desiredKeys[key] {
				// Child belongs to this parent but is no longer in desired set — prune it!
				if item.GetNamespace() != "" {
					_ = c.client.dynClient.Resource(gvr).Namespace(item.GetNamespace()).Delete(c.ctx, item.GetName(), delOpts)
				} else {
					_ = c.client.dynClient.Resource(gvr).Delete(c.ctx, item.GetName(), delOpts)
				}
			}
		}
	}
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
