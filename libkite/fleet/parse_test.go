package fleet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseYAML(t *testing.T) {
	yamlContent := `
- name: node-1
  address: 10.0.0.1
  role: web
  env: production
- name: node-2
  address: 10.0.0.2
  role: db
  env: production
`
	f, err := FromYAML([]byte(yamlContent))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(f.resources))
	}
	if f.resources[0].Name != "node-1" || f.resources[0].Labels["role"] != "web" {
		t.Errorf("unexpected node 0: %+v", f.resources[0])
	}
	if f.resources[1].Name != "node-2" || f.resources[1].Labels["role"] != "db" {
		t.Errorf("unexpected node 1: %+v", f.resources[1])
	}
}

func TestParseGroupedYAML(t *testing.T) {
	groupedYAML := `
web_servers:
  - name: web-1
    address: 10.0.1.1
  - name: web-2
    address: 10.0.1.2
db_servers:
  - name: db-1
    address: 10.0.2.1
`
	f, err := FromYAML([]byte(groupedYAML))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.resources) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(f.resources))
	}

	// Verify group labeling
	webCount := 0
	dbCount := 0
	for _, r := range f.resources {
		if r.Labels["_group"] == "web_servers" {
			webCount++
		}
		if r.Labels["_group"] == "db_servers" {
			dbCount++
		}
	}
	if webCount != 2 || dbCount != 1 {
		t.Errorf("expected 2 web and 1 db, got web=%d db=%d", webCount, dbCount)
	}
}

func TestParseJSONWrapper(t *testing.T) {
	jsonContent := `{
		"hosts": [
			{"name": "h1", "address": "192.168.1.10", "zone": "us-east"},
			{"name": "h2", "address": "192.168.1.11", "zone": "us-west"}
		]
	}`
	f, err := FromJSON([]byte(jsonContent))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.resources) != 2 || f.resources[0].Name != "h1" {
		t.Fatalf("unexpected JSON resources: %+v", f.resources)
	}
}

func TestFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. YAML File
	yamlPath := filepath.Join(tmpDir, "fleet.yaml")
	if err := os.WriteFile(yamlPath, []byte("- name: y1\n  address: 1.1.1.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fy, err := FromFile(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(fy.resources) != 1 || fy.resources[0].Name != "y1" {
		t.Errorf("unexpected yaml file result: %+v", fy.resources)
	}

	// 2. JSON File
	jsonPath := filepath.Join(tmpDir, "fleet.json")
	if err := os.WriteFile(jsonPath, []byte(`[{"name": "j1", "address": "2.2.2.2"}]`), 0644); err != nil {
		t.Fatal(err)
	}
	fj, err := FromFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(fj.resources) != 1 || fj.resources[0].Name != "j1" {
		t.Errorf("unexpected json file result: %+v", fj.resources)
	}

	// 3. Hosts File (POSIX /etc/hosts format)
	hostsPath := filepath.Join(tmpDir, "hosts")
	hostsContent := `# Localhost
127.0.0.1 localhost localhost.localdomain
::1 localhost6

# Cluster nodes
192.168.10.100 picluster-0 picluster-0.local master
192.168.10.101 picluster-1 picluster-1.local worker-1
`
	if err := os.WriteFile(hostsPath, []byte(hostsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Exclude loopback by default
	fh, err := FromHostsFile(hostsPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fh.resources) != 2 {
		t.Fatalf("expected 2 cluster nodes, got %d", len(fh.resources))
	}
	if fh.resources[0].Name != "picluster-0" || fh.resources[0].Address != "192.168.10.100" {
		t.Errorf("unexpected host 0: %+v", fh.resources[0])
	}
	if fh.resources[0].Labels["master"] != "true" {
		t.Errorf("expected master alias label: %+v", fh.resources[0].Labels)
	}

	// Include loopback
	fhAll, err := FromHostsFile(hostsPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fhAll.resources) != 4 {
		t.Fatalf("expected 4 total hosts with loopback, got %d", len(fhAll.resources))
	}
}

func FuzzFleetParse(f *testing.F) {
	f.Add("")
	f.Add("[]")
	f.Add("{}")
	f.Add("- address: 127.0.0.1\n  name: localhost")
	f.Add(`{"hosts": [{"name": "web", "address": "10.0.0.1"}]}`)
	f.Add("web:\n  - 10.0.0.1\n  - 10.0.0.2")
	f.Add("invalid: yaml: : [")
	f.Add("127.0.0.1")

	f.Fuzz(func(t *testing.T, data string) {
		// Invariant 1: Parser must never panic
		f1, err1 := FromYAML([]byte(data))
		f2, err2 := FromJSON([]byte(data))

		if err1 == nil && f1 == nil {
			t.Fatal("expected non-nil fleet on successful YAML parse")
		}
		if err2 == nil && f2 == nil {
			t.Fatal("expected non-nil fleet on successful JSON parse")
		}

		if err1 == nil {
			// Invariant 2: Resulting fleet methods must never panic
			_ = f1.Resources()
			_ = f1.toStarlarkList()
			_ = f1.String()
			_ = f1.Truth()
		}
	})
}
