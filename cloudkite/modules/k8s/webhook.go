package k8s

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	gohttp "net/http"
	"os"
	"sync"
	"os/signal"
	"syscall"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	jsonpatch "gomodules.xyz/jsonpatch/v2"

	"github.com/project-starkite/starkite/starbase"
)

// webhookBuiltin is the k8s.webhook() function that blocks like http.serve().
func (m *Module) webhookBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var validateFn, mutateFn starlark.Callable
	filtered := filterKwargCallable(kwargs, "validate", &validateFn)
	filtered = filterKwargCallable(filtered, "mutate", &mutateFn)

	var p struct {
		Path    string `name:"path" position:"0" required:"true"`
		Port    int    `name:"port"`
		TLSCert string `name:"tls_cert" required:"true"`
		TLSKey  string `name:"tls_key" required:"true"`
	}
	p.Port = 9443
	if err := startype.Args(args, filtered).Go(&p); err != nil {
		return nil, fmt.Errorf("k8s.webhook: %w", err)
	}

	if validateFn == nil && mutateFn == nil {
		return nil, fmt.Errorf("k8s.webhook: at least one handler required (validate or mutate)")
	}

	cert, err := tls.LoadX509KeyPair(p.TLSCert, p.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("k8s.webhook: failed to load TLS cert: %w", err)
	}

	handler := &webhookHandler{
		validateFn: validateFn,
		mutateFn:   mutateFn,
		thread:     thread,
	}

	mux := gohttp.NewServeMux()
	mux.Handle(p.Path, handler)

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
	addr := fmt.Sprintf(":%d", p.Port)
	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("k8s.webhook: listen on %s: %w", addr, err)
	}

	server := &gohttp.Server{Handler: mux}

	// Signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	if _, port, err := net.SplitHostPort(ln.Addr().String()); err == nil {
		fmt.Fprintf(os.Stderr, "k8s.webhook: serving %s on :%s (TLS)\n", p.Path, port)
	}

	if err := server.Serve(ln); err != nil && err != gohttp.ErrServerClosed {
		return nil, fmt.Errorf("k8s.webhook: %w", err)
	}

	return starlark.None, nil
}

// webhookHandler handles AdmissionReview requests.
type webhookHandler struct {
	validateFn starlark.Callable
	mutateFn   starlark.Callable
	thread     *starlark.Thread
}

func (wh *webhookHandler) ServeHTTP(w gohttp.ResponseWriter, r *gohttp.Request) {
	if r.Method != "POST" {
		gohttp.Error(w, "method not allowed", gohttp.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		gohttp.Error(w, "failed to read body", gohttp.StatusBadRequest)
		return
	}

	var review map[string]interface{}
	if err := json.Unmarshal(body, &review); err != nil {
		gohttp.Error(w, "invalid JSON", gohttp.StatusBadRequest)
		return
	}

	request, ok := review["request"].(map[string]interface{})
	if !ok {
		gohttp.Error(w, "missing request field", gohttp.StatusBadRequest)
		return
	}

	uid, ok := request["uid"].(string)
	if !ok || uid == "" {
		gohttp.Error(w, "missing or empty request.uid", gohttp.StatusBadRequest)
		return
	}

	object, ok := request["object"].(map[string]interface{})
	if !ok {
		gohttp.Error(w, "missing request.object", gohttp.StatusBadRequest)
		return
	}

	// Deep-copy object for mutation so original stays unmodified for diffing
	var originalSnapshot map[string]interface{}
	if wh.mutateFn != nil {
		origBytes, err := json.Marshal(object)
		if err != nil {
			gohttp.Error(w, "failed to snapshot object", gohttp.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(origBytes, &originalSnapshot); err != nil {
			gohttp.Error(w, "failed to snapshot object", gohttp.StatusInternalServerError)
			return
		}
	}

	objectAttr := &AttrDict{data: object, mu: &sync.RWMutex{}}

	childThread := &starlark.Thread{Name: "webhook-handler"}
	if wh.thread != nil {
		childThread.Print = wh.thread.Print
	}
	if perms := starbase.GetPermissions(wh.thread); perms != nil {
		starbase.SetPermissions(childThread, perms)
	}

	// Dispatch to the appropriate handler.
	// If both validate and mutate are set, validate runs first — rejection skips mutation.
	var response map[string]interface{}

	if wh.validateFn != nil {
		result, callErr := starlark.Call(childThread, wh.validateFn, starlark.Tuple{objectAttr}, nil)
		response = buildValidationResponse(uid, result, callErr)
		if allowed, ok := response["allowed"].(bool); ok && !allowed {
			// Validation rejected — skip mutation, return immediately
			writeAdmissionResponse(w, response)
			return
		}
	}

	if wh.mutateFn != nil {
		result, callErr := starlark.Call(childThread, wh.mutateFn, starlark.Tuple{objectAttr}, nil)
		response = buildMutationResponse(uid, originalSnapshot, result, callErr)
	}

	writeAdmissionResponse(w, response)
}

func writeAdmissionResponse(w gohttp.ResponseWriter, response map[string]interface{}) {
	reviewResponse := map[string]interface{}{
		"apiVersion": "admission.k8s.io/v1",
		"kind":       "AdmissionReview",
		"response":   response,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reviewResponse); err != nil {
		fmt.Fprintf(os.Stderr, "k8s.webhook: failed to write response: %v\n", err)
	}
}

// ============================================================================
// Response builders
// ============================================================================

func buildValidationResponse(uid string, result starlark.Value, err error) map[string]interface{} {
	resp := map[string]interface{}{"uid": uid}

	if err != nil {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": err.Error()}
		return resp
	}

	dict, ok := result.(*starlark.Dict)
	if !ok {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": fmt.Sprintf("validate handler must return dict, got %s", result.Type())}
		return resp
	}

	allowed, found, getErr := dict.Get(starlark.String("allowed"))
	if getErr != nil || !found {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": "validate handler must return dict with 'allowed' key"}
		return resp
	}

	allowedBool, ok := allowed.(starlark.Bool)
	if !ok {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": fmt.Sprintf("'allowed' must be bool, got %s", allowed.Type())}
		return resp
	}

	resp["allowed"] = bool(allowedBool)

	if msg, found, getErr := dict.Get(starlark.String("message")); getErr == nil && found {
		if s, ok := starlark.AsString(msg); ok && s != "" {
			resp["status"] = map[string]interface{}{"message": s}
		}
	}

	return resp
}

func buildMutationResponse(uid string, original map[string]interface{}, result starlark.Value, err error) map[string]interface{} {
	resp := map[string]interface{}{"uid": uid, "allowed": true}

	if err != nil {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": err.Error()}
		return resp
	}

	var modified map[string]interface{}
	switch v := result.(type) {
	case *AttrDict:
		modified = v.data
	case *starlark.Dict:
		var goVal interface{}
		if convErr := startype.Starlark(v).Go(&goVal); convErr != nil {
			resp["allowed"] = false
			resp["status"] = map[string]interface{}{"message": fmt.Sprintf("mutate handler return conversion error: %v", convErr)}
			return resp
		}
		if m, ok := goVal.(map[string]interface{}); ok {
			modified = m
		}
	default:
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": fmt.Sprintf("mutate handler must return dict or AttrDict, got %s", result.Type())}
		return resp
	}

	if modified == nil {
		return resp
	}

	// Generate RFC 6902 JSON patch using gomodules.xyz/jsonpatch/v2
	originalBytes, err := json.Marshal(original)
	if err != nil {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": fmt.Sprintf("failed to marshal original: %v", err)}
		return resp
	}

	modifiedBytes, err := json.Marshal(modified)
	if err != nil {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": fmt.Sprintf("failed to marshal modified: %v", err)}
		return resp
	}

	patches, err := jsonpatch.CreatePatch(originalBytes, modifiedBytes)
	if err != nil {
		resp["allowed"] = false
		resp["status"] = map[string]interface{}{"message": fmt.Sprintf("failed to generate patch: %v", err)}
		return resp
	}

	if len(patches) > 0 {
		patchBytes, marshalErr := json.Marshal(patches)
		if marshalErr != nil {
			resp["allowed"] = false
			resp["status"] = map[string]interface{}{"message": fmt.Sprintf("failed to marshal patches: %v", marshalErr)}
			return resp
		}
		resp["patchType"] = "JSONPatch"
		resp["patch"] = base64.StdEncoding.EncodeToString(patchBytes)
	}

	return resp
}
