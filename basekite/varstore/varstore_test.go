package varstore

import (
	"reflect"
	"testing"
)

func TestParseConfigFile_Permissions(t *testing.T) {
	v := New()
	cfg := []byte(`permissions:
  default:
    allow: [fs.read, os.env]
  deploy:
    allow: [fs.read, fs.write, "os.exec($CWD/**)"]
    deny: [fs.delete]
`)
	if err := v.parseConfigFile(cfg); err != nil {
		t.Fatalf("parseConfigFile: %v", err)
	}

	def, ok := v.Permissions["default"]
	if !ok {
		t.Fatal("default profile not parsed")
	}
	if !reflect.DeepEqual(def.Allow, []string{"fs.read", "os.env"}) {
		t.Errorf("default.allow = %v", def.Allow)
	}

	deploy, ok := v.Permissions["deploy"]
	if !ok {
		t.Fatal("deploy profile not parsed")
	}
	if !reflect.DeepEqual(deploy.Allow, []string{"fs.read", "fs.write", "os.exec($CWD/**)"}) {
		t.Errorf("deploy.allow = %v", deploy.Allow)
	}
	if !reflect.DeepEqual(deploy.Deny, []string{"fs.delete"}) {
		t.Errorf("deploy.deny = %v", deploy.Deny)
	}
}

func TestParseConfigFile_ThreeSections(t *testing.T) {
	v := New()
	cfg := []byte(`config:
  project:
    name: my-project
  defaults:
    log_level: info
  providers:
    ssh:
      user: deploy
  active_edition: cloud
  environment: dev
  labels:
    app: myapp
permissions:
  ci:
    allow: [fs.read]
sandbox:
  mybox:
    network: host
`)
	if err := v.parseConfigFile(cfg); err != nil {
		t.Fatalf("parseConfigFile: %v", err)
	}

	// Runtime keys parsed into their sections, not variables.
	if v.ProjectConfig["name"] != "my-project" {
		t.Errorf("project.name = %v", v.ProjectConfig["name"])
	}
	if v.RuntimeDefaults["log_level"] != "info" {
		t.Errorf("defaults.log_level = %v", v.RuntimeDefaults["log_level"])
	}
	if v.ProviderDefaults["ssh"]["user"] != "deploy" {
		t.Errorf("providers.ssh.user = %v", v.ProviderDefaults["ssh"])
	}
	if v.ActiveEdition != "cloud" {
		t.Errorf("active_edition = %q", v.ActiveEdition)
	}

	// User keys under config: flatten into variables.
	if got := v.defaultVars["environment"]; got != "dev" {
		t.Errorf("environment var = %v", got)
	}
	if got := v.defaultVars["labels.app"]; got != "myapp" {
		t.Errorf("labels.app var = %v", got)
	}

	// permissions: parsed; sandbox: tolerated (consumed elsewhere).
	if _, ok := v.Permissions["ci"]; !ok {
		t.Error("permissions.ci not parsed")
	}
}

func TestParseConfigFile_PermissionAlias(t *testing.T) {
	v := New()
	cfg := []byte(`permissions:
  default: allow-all
  ci:
    allow: [fs.read]
`)
	if err := v.parseConfigFile(cfg); err != nil {
		t.Fatalf("parseConfigFile: %v", err)
	}
	if v.Permissions["default"].Alias != "allow-all" {
		t.Errorf("default.Alias = %q, want allow-all", v.Permissions["default"].Alias)
	}
	if len(v.Permissions["ci"].Allow) != 1 {
		t.Errorf("ci profile not parsed: %+v", v.Permissions["ci"])
	}
}

func TestParseConfigFile_UnknownTopLevelKeyErrors(t *testing.T) {
	v := New()
	// Loose top-level user variables are rejected: they belong under config:.
	if err := v.parseConfigFile([]byte("environment: dev\n")); err == nil {
		t.Fatal("expected error for unknown top-level key")
	}
}

func TestTryParseJSON_PlainString(t *testing.T) {
	result := tryParseJSON("hello")
	if result != "hello" {
		t.Fatalf("expected 'hello', got %v", result)
	}
}

func TestTryParseJSON_EmptyString(t *testing.T) {
	result := tryParseJSON("")
	if result != "" {
		t.Fatalf("expected empty string, got %v", result)
	}
}

func TestTryParseJSON_Array(t *testing.T) {
	result := tryParseJSON(`[1, 2, 3]`)
	slice, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(slice) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(slice))
	}
}

func TestTryParseJSON_Object(t *testing.T) {
	result := tryParseJSON(`{"app": "web"}`)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["app"] != "web" {
		t.Fatalf("expected 'web', got %v", m["app"])
	}
}

func TestTryParseJSON_InvalidJSON(t *testing.T) {
	result := tryParseJSON(`[not valid`)
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "[not valid" {
		t.Fatalf("expected original string, got %v", s)
	}
}

func TestTryParseJSON_WithWhitespace(t *testing.T) {
	result := tryParseJSON(`  [1, 2]  `)
	slice, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(slice) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(slice))
	}
}

func TestKeys_Sorted(t *testing.T) {
	v := New()
	v.cliVars["zebra"] = "z"
	v.cliVars["alpha"] = "a"
	v.cliVars["middle"] = "m"

	keys := v.Keys()
	expected := []string{"alpha", "middle", "zebra"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Fatalf("expected keys[%d]=%q, got %q", i, expected[i], k)
		}
	}
}

func TestKeys_Deduplicated(t *testing.T) {
	v := New()
	v.cliVars["shared"] = "cli"
	v.envVars["shared"] = "env"
	v.fileVars["unique"] = "file"

	keys := v.Keys()
	// "shared" should appear only once
	expected := []string{"shared", "unique"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
	}
}

func TestKeys_Empty(t *testing.T) {
	v := New()
	keys := v.Keys()
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestFlattenAndStore_PreservesMap(t *testing.T) {
	v := New()
	store := make(map[string]interface{})
	input := map[string]interface{}{
		"app": "web",
		"env": "prod",
	}
	v.flattenAndStore("labels", input, store)

	// Check the original map is preserved at the prefix key
	m, ok := store["labels"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map at 'labels', got %T", store["labels"])
	}
	if m["app"] != "web" {
		t.Fatalf("expected 'web', got %v", m["app"])
	}

	// Check flattened keys also exist
	if store["labels.app"] != "web" {
		t.Fatalf("expected 'web' at 'labels.app', got %v", store["labels.app"])
	}
	if store["labels.env"] != "prod" {
		t.Fatalf("expected 'prod' at 'labels.env', got %v", store["labels.env"])
	}
}

func TestFlattenAndStore_NestedMaps(t *testing.T) {
	v := New()
	store := make(map[string]interface{})
	input := map[string]interface{}{
		"resources": map[string]interface{}{
			"cpu":    "500m",
			"memory": "256Mi",
		},
	}
	v.flattenAndStore("config", input, store)

	// Top-level map preserved
	_, ok := store["config"].(map[string]interface{})
	if !ok {
		t.Fatal("expected map at 'config'")
	}

	// Nested map preserved
	_, ok = store["config.resources"].(map[string]interface{})
	if !ok {
		t.Fatal("expected map at 'config.resources'")
	}

	// Leaf values accessible
	if store["config.resources.cpu"] != "500m" {
		t.Fatalf("expected '500m', got %v", store["config.resources.cpu"])
	}
}

func TestLoadFromCLI_JSONDetection(t *testing.T) {
	v := New()
	err := v.LoadFromCLI([]string{
		`ports=[8080, 8443]`,
		`labels={"app":"web"}`,
		`name=myapp`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// JSON array should be parsed
	ports, ok := v.Get("ports")
	if !ok {
		t.Fatal("expected 'ports' to exist")
	}
	if _, ok := ports.([]interface{}); !ok {
		t.Fatalf("expected []interface{} for ports, got %T", ports)
	}

	// JSON object should be parsed
	labels, ok := v.Get("labels")
	if !ok {
		t.Fatal("expected 'labels' to exist")
	}
	if _, ok := labels.(map[string]interface{}); !ok {
		t.Fatalf("expected map[string]interface{} for labels, got %T", labels)
	}

	// Plain string should stay as string
	name, ok := v.Get("name")
	if !ok {
		t.Fatal("expected 'name' to exist")
	}
	if name != "myapp" {
		t.Fatalf("expected 'myapp', got %v", name)
	}
}
