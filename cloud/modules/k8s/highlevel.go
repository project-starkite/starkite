package k8s

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// deployHighLevel creates a Deployment and optional Service in one call.
// Signature: k8s.deploy(name, image, replicas=1, port=0, namespace="", labels=None, env=None, timeout="")
func (c *K8sClient) deployHighLevel(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// labels and env are *starlark.Dict — extract from kwargs before startype
	var labelsDict, envDict *starlark.Dict
	filteredKwargs := filterKwarg(kwargs, "labels", &labelsDict)
	filteredKwargs = filterKwarg(filteredKwargs, "env", &envDict)

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Image     string `name:"image" position:"1" required:"true"`
		Replicas  int    `name:"replicas"`
		Port      int    `name:"port"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Replicas == 0 {
		p.Replicas = 1
	}
	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	labels := map[string]any{"app": p.Name}
	if labelsDict != nil {
		for _, item := range labelsDict.Items() {
			k, _ := starlark.AsString(item[0])
			v, _ := starlark.AsString(item[1])
			labels[k] = v
		}
	}

	// Build container spec
	container := map[string]any{
		"name":  p.Name,
		"image": p.Image,
	}
	if p.Port > 0 {
		container["ports"] = []any{
			map[string]any{"containerPort": int64(p.Port)},
		}
	}
	if envDict != nil {
		var envVars []any
		for _, item := range envDict.Items() {
			k, _ := starlark.AsString(item[0])
			v, _ := starlark.AsString(item[1])
			envVars = append(envVars, map[string]any{"name": k, "value": v})
		}
		container["env"] = envVars
	}

	// Build Deployment manifest
	deploy := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      p.Name,
			"namespace": ns,
			"labels":    labels,
		},
		"spec": map[string]any{
			"replicas": int64(p.Replicas),
			"selector": map[string]any{
				"matchLabels": labels,
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": labels,
				},
				"spec": map[string]any{
					"containers": []any{container},
				},
			},
		},
	}

	deployData, err := json.Marshal(deploy)
	if err != nil {
		return nil, fmt.Errorf("k8s.deploy: marshal deployment: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.deploy: %w", err)
	}
	defer cancel()

	force := true
	applyOpts := metav1.PatchOptions{
		FieldManager: "starkite",
		Force:        &force,
	}

	gvr, _, _ := c.resolver.Resolve("deployment")
	_, err = c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.ApplyPatchType, deployData, applyOpts)
	if err != nil {
		return nil, fmt.Errorf("k8s.deploy: apply deployment: %w", err)
	}

	result := starlark.NewDict(2)
	result.SetKey(starlark.String("deployment"), starlark.String(p.Name))

	// Create Service if port specified
	if p.Port > 0 {
		svc := map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]any{
				"name":      p.Name,
				"namespace": ns,
			},
			"spec": map[string]any{
				"selector": labels,
				"ports": []any{
					map[string]any{
						"port":       int64(p.Port),
						"targetPort": int64(p.Port),
						"protocol":   "TCP",
					},
				},
			},
		}

		svcData, err := json.Marshal(svc)
		if err != nil {
			return nil, fmt.Errorf("k8s.deploy: marshal service: %w", err)
		}

		svcGVR, _, _ := c.resolver.Resolve("service")
		_, err = c.dynClient.Resource(svcGVR).Namespace(ns).Patch(ctx, p.Name, types.ApplyPatchType, svcData, applyOpts)
		if err != nil {
			return nil, fmt.Errorf("k8s.deploy: apply service: %w", err)
		}
		result.SetKey(starlark.String("service"), starlark.String(p.Name))
	}

	return result, nil
}

// run creates and runs a Pod, optionally waiting for completion and returning logs.
// Signature: k8s.run(name, image, command=None, namespace="", restart="Never", rm=False, timeout="3m")
func (c *K8sClient) run(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// command is starlark.Value at position 2 — extract before startype
	var command starlark.Value
	filteredKwargs := filterKwargValue(kwargs, "command", &command)
	remaining := args
	if command == nil && len(args) > 2 {
		command = args[2]
		remaining = args[:2]
	}

	var p struct {
		Name      string `name:"name" position:"0" required:"true"`
		Image     string `name:"image" position:"1" required:"true"`
		Namespace string `name:"namespace"`
		Restart   string `name:"restart"`
		Rm        bool   `name:"rm"`
		Timeout   string `name:"timeout"`
	}
	p.Timeout = "3m" // default
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}
	restart := p.Restart
	if restart == "" {
		restart = "Never"
	}

	containerSpec := map[string]any{
		"name":  p.Name,
		"image": p.Image,
	}

	if command != nil {
		switch v := command.(type) {
		case starlark.String:
			containerSpec["command"] = []any{"/bin/sh", "-c", string(v)}
		case *starlark.List:
			var cmd []any
			for i := 0; i < v.Len(); i++ {
				s, _ := starlark.AsString(v.Index(i))
				cmd = append(cmd, s)
			}
			containerSpec["command"] = cmd
		}
	}

	pod := &unstructuredObj{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      p.Name,
			"namespace": ns,
		},
		"spec": map[string]any{
			"containers":    []any{containerSpec},
			"restartPolicy": restart,
		},
	}}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.run: %w", err)
	}
	defer cancel()

	podGVR, _, _ := c.resolver.Resolve("pod")

	created, err := c.dynClient.Resource(podGVR).Namespace(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.run: create pod: %w", err)
	}

	// Wait for completion if restart=Never
	if restart == "Never" {
		for {
			select {
			case <-ctx.Done():
				goto done
			default:
			}
			obj, err := c.dynClient.Resource(podGVR).Namespace(ns).Get(ctx, p.Name, metav1.GetOptions{})
			if err != nil {
				break
			}
			phase, _, _ := unstructuredNestedString(obj.Object, "status", "phase")
			if phase == "Succeeded" || phase == "Failed" {
				break
			}
			time.Sleep(2 * time.Second)
		}
	}
done:

	result := starlark.NewDict(2)
	result.SetKey(starlark.String("name"), starlark.String(created.GetName()))

	// Get logs
	logVal, err := c.logs(thread, fn, starlark.Tuple{starlark.String(p.Name)}, []starlark.Tuple{
		{starlark.String("namespace"), starlark.String(ns)},
	})
	if err == nil {
		result.SetKey(starlark.String("logs"), logVal)
	}

	// Cleanup if rm=True
	if p.Rm {
		c.dynClient.Resource(podGVR).Namespace(ns).Delete(ctx, p.Name, metav1.DeleteOptions{})
	}

	return result, nil
}

// expose creates a Service to expose a resource.
// Signature: k8s.expose(kind, name, port, target_port=0, type="ClusterIP", namespace="", timeout="")
func (c *K8sClient) expose(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind       string `name:"kind" position:"0" required:"true"`
		Name       string `name:"name" position:"1" required:"true"`
		Port       int    `name:"port" position:"2" required:"true"`
		TargetPort int    `name:"target_port"`
		Type       string `name:"type"`
		Namespace  string `name:"namespace"`
		Timeout    string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}
	svcType := p.Type
	if svcType == "" {
		svcType = "ClusterIP"
	}
	targetPort := p.TargetPort
	if targetPort == 0 {
		targetPort = p.Port
	}

	svc := &unstructuredObj{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name":      p.Name,
			"namespace": ns,
		},
		"spec": map[string]any{
			"type":     svcType,
			"selector": map[string]any{"app": p.Name},
			"ports": []any{
				map[string]any{
					"port":       int64(p.Port),
					"targetPort": int64(targetPort),
					"protocol":   "TCP",
				},
			},
		},
	}}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.expose: %w", err)
	}
	defer cancel()

	svcGVR, _, _ := c.resolver.Resolve("service")

	created, err := c.dynClient.Resource(svcGVR).Namespace(ns).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.expose: %w", err)
	}

	return unstructuredToDict(created)
}

// scale changes the replica count for a scalable resource.
// Signature: k8s.scale(kind, name, replicas, namespace="", timeout="")
func (c *K8sClient) scale(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Replicas  int    `name:"replicas" position:"2" required:"true"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	gvr, namespaced, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.scale: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	patchObj := map[string]any{
		"spec": map[string]any{
			"replicas": int64(p.Replicas),
		},
	}
	patchBytes, err := json.Marshal(patchObj)
	if err != nil {
		return nil, fmt.Errorf("k8s.scale: marshal: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.scale: %w", err)
	}
	defer cancel()

	var patched *unstructuredObj
	if namespaced {
		patched, err = c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	} else {
		patched, err = c.dynClient.Resource(gvr).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("k8s.scale: %w", err)
	}

	return unstructuredToDict(patched)
}

// autoscale creates a HorizontalPodAutoscaler.
// Signature: k8s.autoscale(kind, name, min=1, max=10, cpu_percent=80, namespace="", timeout="")
func (c *K8sClient) autoscale(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind       string `name:"kind" position:"0" required:"true"`
		Name       string `name:"name" position:"1" required:"true"`
		Min        int    `name:"min"`
		Max        int    `name:"max"`
		CpuPercent int    `name:"cpu_percent"`
		Namespace  string `name:"namespace"`
		Timeout    string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Min == 0 {
		p.Min = 1
	}
	if p.Max == 0 {
		p.Max = 10
	}
	if p.CpuPercent == 0 {
		p.CpuPercent = 80
	}
	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	// Resolve the target kind to get the API group
	gvr, _, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.autoscale: %w", err)
	}

	apiVersion := gvr.Group + "/" + gvr.Version
	if gvr.Group == "" {
		apiVersion = gvr.Version
	}

	hpa := &unstructuredObj{Object: map[string]any{
		"apiVersion": "autoscaling/v2",
		"kind":       "HorizontalPodAutoscaler",
		"metadata": map[string]any{
			"name":      p.Name + "-hpa",
			"namespace": ns,
		},
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{
				"apiVersion": apiVersion,
				"kind":       gvr.Resource,
				"name":       p.Name,
			},
			"minReplicas": int64(p.Min),
			"maxReplicas": int64(p.Max),
			"metrics": []any{
				map[string]any{
					"type": "Resource",
					"resource": map[string]any{
						"name": "cpu",
						"target": map[string]any{
							"type":               "Utilization",
							"averageUtilization": int64(p.CpuPercent),
						},
					},
				},
			},
		},
	}}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.autoscale: %w", err)
	}
	defer cancel()

	hpaGVR, _, _ := c.resolver.Resolve("hpa")

	created, err := c.dynClient.Resource(hpaGVR).Namespace(ns).Create(ctx, hpa, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.autoscale: %w", err)
	}

	return unstructuredToDict(created)
}

// rollout manages deployment rollouts.
// Signature: k8s.rollout(kind, name, action="status", namespace="", revision=0, timeout="")
func (c *K8sClient) rollout(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Action    string `name:"action"`
		Namespace string `name:"namespace"`
		Revision  int    `name:"revision"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.Action == "" {
		p.Action = "status"
	}
	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	gvr, _, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.rollout: %w", err)
	}

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.rollout: %w", err)
	}
	defer cancel()

	switch p.Action {
	case "status":
		obj, err := c.dynClient.Resource(gvr).Namespace(ns).Get(ctx, p.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("k8s.rollout: %w", err)
		}
		return rolloutStatusFromObj(obj)

	case "restart":
		patchObj := map[string]any{
			"spec": map[string]any{
				"template": map[string]any{
					"metadata": map[string]any{
						"annotations": map[string]any{
							"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
						},
					},
				},
			},
		}
		patchBytes, _ := json.Marshal(patchObj)
		patched, err := c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			return nil, fmt.Errorf("k8s.rollout restart: %w", err)
		}
		return unstructuredToDict(patched)

	case "pause":
		patchObj := map[string]any{
			"spec": map[string]any{"paused": true},
		}
		patchBytes, _ := json.Marshal(patchObj)
		patched, err := c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			return nil, fmt.Errorf("k8s.rollout pause: %w", err)
		}
		return unstructuredToDict(patched)

	case "resume":
		patchObj := map[string]any{
			"spec": map[string]any{"paused": false},
		}
		patchBytes, _ := json.Marshal(patchObj)
		patched, err := c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			return nil, fmt.Errorf("k8s.rollout resume: %w", err)
		}
		return unstructuredToDict(patched)

	default:
		return nil, fmt.Errorf("k8s.rollout: unknown action %q (use status, restart, pause, or resume)", p.Action)
	}
}

func rolloutStatusFromObj(obj *unstructuredObj) (starlark.Value, error) {
	result := starlark.NewDict(5)

	replicas, _, _ := nestedField(obj.Object, "status", "replicas")
	ready, _, _ := nestedField(obj.Object, "status", "readyReplicas")
	updated, _, _ := nestedField(obj.Object, "status", "updatedReplicas")
	available, _, _ := nestedField(obj.Object, "status", "availableReplicas")

	toInt := func(v any) starlark.Value {
		switch n := v.(type) {
		case int64:
			return starlark.MakeInt64(n)
		case float64:
			return starlark.MakeInt(int(n))
		default:
			return starlark.MakeInt(0)
		}
	}

	result.SetKey(starlark.String("replicas"), toInt(replicas))
	result.SetKey(starlark.String("ready"), toInt(ready))
	result.SetKey(starlark.String("updated"), toInt(updated))
	result.SetKey(starlark.String("available"), toInt(available))

	// Check if rollout is complete
	complete := replicas == ready && replicas == updated && replicas == available
	result.SetKey(starlark.String("complete"), starlark.Bool(complete))

	return result, nil
}

// setImage updates the container image on a workload.
// Signature: k8s.set_image(kind, name, container, image, namespace="", timeout="")
func (c *K8sClient) setImage(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Container string `name:"container" position:"2" required:"true"`
		Image     string `name:"image" position:"3" required:"true"`
		Namespace string `name:"namespace"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	gvr, _, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.set_image: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	// Strategic merge patch to update a specific container's image
	patchObj := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"name":  p.Container,
							"image": p.Image,
						},
					},
				},
			},
		},
	}
	patchBytes, _ := json.Marshal(patchObj)

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.set_image: %w", err)
	}
	defer cancel()

	patched, err := c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.set_image: %w", err)
	}

	return unstructuredToDict(patched)
}

// setEnv updates environment variables on a workload.
// Signature: k8s.set_env(kind, name, env, namespace="", container="", timeout="")
func (c *K8sClient) setEnv(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// env is *starlark.Dict at position 2 — extract before startype
	var envDict *starlark.Dict
	filteredKwargs := filterKwarg(kwargs, "env", &envDict)
	remaining := args
	if envDict == nil && len(args) > 2 {
		if d, ok := args[2].(*starlark.Dict); ok {
			envDict = d
		}
		remaining = args[:2]
	}

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Namespace string `name:"namespace"`
		Container string `name:"container"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(remaining, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}
	if envDict == nil {
		return nil, fmt.Errorf("k8s.set_env: missing required argument: env")
	}

	gvr, _, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.set_env: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	var envVars []any
	for _, item := range envDict.Items() {
		k, _ := starlark.AsString(item[0])
		v, _ := starlark.AsString(item[1])
		envVars = append(envVars, map[string]any{"name": k, "value": v})
	}

	containerPatch := map[string]any{"env": envVars}
	if p.Container != "" {
		containerPatch["name"] = p.Container
	}

	patchObj := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{containerPatch},
				},
			},
		},
	}
	patchBytes, _ := json.Marshal(patchObj)

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.set_env: %w", err)
	}
	defer cancel()

	patched, err := c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.set_env: %w", err)
	}

	return unstructuredToDict(patched)
}

// setResources updates resource requests/limits on a workload.
// Signature: k8s.set_resources(kind, name, requests=None, limits=None, namespace="", container="", timeout="")
func (c *K8sClient) setResources(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "k8s", "write", ""); err != nil {
		return nil, err
	}

	// requests and limits are *starlark.Dict — extract from kwargs before startype
	var requestsDict, limitsDict *starlark.Dict
	filteredKwargs := filterKwarg(kwargs, "requests", &requestsDict)
	filteredKwargs = filterKwarg(filteredKwargs, "limits", &limitsDict)

	var p struct {
		Kind      string `name:"kind" position:"0" required:"true"`
		Name      string `name:"name" position:"1" required:"true"`
		Namespace string `name:"namespace"`
		Container string `name:"container"`
		Timeout   string `name:"timeout"`
	}
	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	gvr, _, err := c.resolver.Resolve(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("k8s.set_resources: %w", err)
	}

	ns := p.Namespace
	if ns == "" {
		ns = c.namespace
	}

	resources := map[string]any{}
	if requestsDict != nil {
		reqs := map[string]any{}
		for _, item := range requestsDict.Items() {
			k, _ := starlark.AsString(item[0])
			v, _ := starlark.AsString(item[1])
			reqs[k] = v
		}
		resources["requests"] = reqs
	}
	if limitsDict != nil {
		lims := map[string]any{}
		for _, item := range limitsDict.Items() {
			k, _ := starlark.AsString(item[0])
			v, _ := starlark.AsString(item[1])
			lims[k] = v
		}
		resources["limits"] = lims
	}

	containerPatch := map[string]any{"resources": resources}
	if p.Container != "" {
		containerPatch["name"] = p.Container
	}

	patchObj := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{containerPatch},
				},
			},
		},
	}
	patchBytes, _ := json.Marshal(patchObj)

	ctx, cancel, err := c.contextWithTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("k8s.set_resources: %w", err)
	}
	defer cancel()

	patched, err := c.dynClient.Resource(gvr).Namespace(ns).Patch(ctx, p.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s.set_resources: %w", err)
	}

	return unstructuredToDict(patched)
}
