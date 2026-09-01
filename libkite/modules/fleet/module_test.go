package fleet

import (
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestFleetModule(t *testing.T) {
	tmpDir := t.TempDir()
	hostsFile := filepath.Join(tmpDir, "hosts.yaml")
	yamlContent := `
- name: node-1
  address: 192.168.1.10
  role: web
- name: node-2
  address: 192.168.1.11
  role: db
`
	if err := os.WriteFile(hostsFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	posixHostsFile := filepath.Join(tmpDir, "hosts")
	posixContent := `
127.0.0.1 localhost
192.168.10.100 pi-0 pi-0.local master
192.168.10.101 pi-1 pi-1.local worker
`
	if err := os.WriteFile(posixHostsFile, []byte(posixContent), 0644); err != nil {
		t.Fatal(err)
	}

	script := `
def test():
    # 1. Test fleet.file() and fleet.new(file=...)
    f1 = fleet.file("` + filepath.ToSlash(hostsFile) + `")
    if f1.count != 2:
        return "failed f1 count"
    
    f1_kw = fleet.new(file="` + filepath.ToSlash(hostsFile) + `")
    if f1_kw.count != 2:
        return "failed f1_kw count"

    web_nodes = f1.filter(role="web")
    if web_nodes.count != 1:
        return "failed web filter"
    if web_nodes.addresses()[0] != "192.168.1.10":
        return "failed address extraction"

    # 2. Test fleet.new() with list
    f2 = fleet.new([
        {"name": "worker-1", "address": "10.0.0.1", "zone": "east"},
        {"name": "worker-2", "address": "10.0.0.2", "zone": "west"},
    ])
    if f2.count != 2:
        return "failed f2 count"

    # 3. Test fleet.new(function=...)
    def discover():
        return [{"name": "dyn-1", "address": "172.16.0.1"}]
    
    f3 = fleet.new(function=discover)
    if f3.count != 1 or f3.first()["name"] != "dyn-1":
        return "failed f3 discovery"

    # 4. Test fleet.new(json=...)
    f4 = fleet.new(json='[{"name": "json-1", "address": "10.1.1.1"}]')
    if f4.count != 1 or f4.first()["name"] != "json-1":
        return "failed f4 json"

    # 5. Test fleet.hosts_file()
    f5 = fleet.hosts_file("` + filepath.ToSlash(posixHostsFile) + `")
    if f5.count != 2:
        return "failed f5 count: " + str(f5.count)
    if f5.first()["name"] != "pi-0" or f5.first()["address"] != "192.168.10.100":
        return "failed f5 pi-0"

    # 6. Test fleet.new(hosts_file=...)
    f6 = fleet.new(hosts_file="` + filepath.ToSlash(posixHostsFile) + `", loopback=True)
    if f6.count != 3:
        return "failed f6 count with loopback: " + str(f6.count)

    return "ok"
`

	rt, err := libkite.New(&libkite.Config{
		Permissions: libkite.AllowAllPermissions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	mod := New()
	dict, err := mod.Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	predeclared := starlark.StringDict{
		"fleet": dict["fleet"],
	}

	thread := rt.NewThread("test-fleet-module")
	globals, err := starlark.ExecFile(thread, "test.star", script, predeclared)
	if err != nil {
		t.Fatal(err)
	}

	testFn, ok := globals["test"]
	if !ok {
		t.Fatal("test function not found")
	}

	res, err := starlark.Call(thread, testFn, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := starlark.AsString(res)
	if !ok || got != "ok" {
		t.Fatalf("test returned %v, want 'ok'", res)
	}
}
