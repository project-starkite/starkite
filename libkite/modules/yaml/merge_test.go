package yaml

import (
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
)

func evalYAMLStarlark(t *testing.T, script string) starlark.StringDict {
	t.Helper()
	m := newTestModule(t)
	globals, err := m.Load(m.config)
	if err != nil {
		t.Fatal(err)
	}
	thread := &starlark.Thread{Name: "test-yaml-merge"}
	out, err := starlark.ExecFile(thread, "test.star", script, globals)
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	return out
}

func TestMergeRecursiveAndNoneDelete(t *testing.T) {
	script := `
base = {
    "service": "api",
    "debug": True,
    "db": {"host": "localhost", "port": 5432, "ssl": False},
    "tags": ["a", "b"],
}
override = {
    "debug": None,
    "db": {"host": "prod.db.internal", "ssl": True},
    "tags": ["c"],
}
res = yaml.merge(base, override)
`
	out := evalYAMLStarlark(t, script)
	res := out["res"].(*starlark.Dict)
	base := out["base"].(*starlark.Dict)

	// Base must remain unmutated.
	if _, found, _ := base.Get(starlark.String("debug")); !found {
		t.Fatal("base dict was mutated: debug key missing")
	}
	if _, found, _ := res.Get(starlark.String("debug")); found {
		t.Fatal("expected debug key to be deleted by None override")
	}

	dbVal, _, _ := res.Get(starlark.String("db"))
	db := dbVal.(*starlark.Dict)
	host, _, _ := db.Get(starlark.String("host"))
	port, _, _ := db.Get(starlark.String("port"))
	if string(host.(starlark.String)) != "prod.db.internal" {
		t.Fatalf("db.host = %v, want prod.db.internal", host)
	}
	if port.(starlark.Int).String() != "5432" {
		t.Fatalf("db.port = %v, want 5432", port)
	}

	tagsVal, _, _ := res.Get(starlark.String("tags"))
	tags := tagsVal.(*starlark.List)
	if tags.Len() != 1 || string(tags.Index(0).(starlark.String)) != "c" {
		t.Fatalf("tags = %v, want ['c']", tags)
	}
}

func TestMergeListModes(t *testing.T) {
	script := `
base = {
    "items": ["a", "b"],
    "containers": [
        {"name": "app", "image": "app:1", "env": [{"name": "LOG", "value": "info"}]},
        {"name": "sidecar", "image": "proxy:1"},
    ],
}
patch_append = {"items": ["c"]}
res_append = yaml.merge(base, patch_append, list_mode="append")

patch_keyed = {
    "containers": [
        {"name": "app", "image": "app:2", "env": [{"name": "LOG", "value": "debug"}, {"name": "MODE", "value": "prod"}]},
        {"name": "metrics", "image": "prom:1"},
    ],
}
res_keyed = yaml.merge(base, patch_keyed, list_mode="by_key", list_key="name")
`
	out := evalYAMLStarlark(t, script)

	appended := out["res_append"].(*starlark.Dict)
	itemsVal, _, _ := appended.Get(starlark.String("items"))
	items := itemsVal.(*starlark.List)
	if items.Len() != 3 {
		t.Fatalf("append items len = %d, want 3", items.Len())
	}

	keyed := out["res_keyed"].(*starlark.Dict)
	cVal, _, _ := keyed.Get(starlark.String("containers"))
	containers := cVal.(*starlark.List)
	if containers.Len() != 3 {
		t.Fatalf("keyed containers len = %d, want 3", containers.Len())
	}

	c0 := containers.Index(0).(*starlark.Dict)
	img, _, _ := c0.Get(starlark.String("image"))
	if string(img.(starlark.String)) != "app:2" {
		t.Fatalf("containers[0].image = %v, want app:2", img)
	}
	envVal, _, _ := c0.Get(starlark.String("env"))
	envList := envVal.(*starlark.List)
	if envList.Len() != 2 {
		t.Fatalf("containers[0].env len = %d, want 2", envList.Len())
	}

	c1 := containers.Index(1).(*starlark.Dict)
	c1Name, _, _ := c1.Get(starlark.String("name"))
	if string(c1Name.(starlark.String)) != "sidecar" {
		t.Fatalf("containers[1].name = %v, want sidecar", c1Name)
	}
}

func TestMergeYAMLStringsAndFilesAndFrozen(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.yaml")
	if err := os.WriteFile(basePath, []byte("name: demo\nreplicas: 1\nlabels:\n  app: hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := `
res = yaml.merge(
    yaml.file("` + filepath.ToSlash(basePath) + `"),
    "replicas: 3\nlabels:\n  env: prod\n",
    {"labels": {"tier": "web"}},
)
`
	out := evalYAMLStarlark(t, script)
	res := out["res"].(*starlark.Dict)

	replicas, _, _ := res.Get(starlark.String("replicas"))
	if replicas.(starlark.Int).String() != "3" {
		t.Fatalf("replicas = %v, want 3", replicas)
	}
	labelsVal, _, _ := res.Get(starlark.String("labels"))
	labels := labelsVal.(*starlark.Dict)
	if labels.Len() != 3 {
		t.Fatalf("labels len = %d, want 3 (%v)", labels.Len(), labels)
	}
}

func TestMergeErrorsAndTryVariant(t *testing.T) {
	script := `
r1 = yaml.try_merge({"a": 1}, {"b": 2}, list_mode="by_key")
r2 = yaml.try_merge({"a": 1}, {"b": 2}, list_mode="bogus")
r3 = yaml.try_merge("not-a-map")
r4 = yaml.try_merge({"a": 1}, {"b": 2})
`
	out := evalYAMLStarlark(t, script)
	for _, name := range []string{"r1", "r2", "r3"} {
		res := out[name].(starlark.HasAttrs)
		okVal, err := res.Attr("ok")
		if err != nil {
			t.Fatal(err)
		}
		if bool(okVal.(starlark.Bool)) {
			t.Fatalf("%s: expected ok=False, got True", name)
		}
	}
	r4 := out["r4"].(starlark.HasAttrs)
	okVal, err := r4.Attr("ok")
	if err != nil {
		t.Fatal(err)
	}
	if !bool(okVal.(starlark.Bool)) {
		t.Fatalf("r4: expected ok=True, got False")
	}
}
