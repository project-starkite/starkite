package fleet

import (
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func TestResourceNormalization(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		wantAddress string
		wantName    string
		wantKind    string
		wantRole    string
		wantErr     bool
	}{
		{
			name:        "plain string IP",
			input:       "192.168.1.10",
			wantAddress: "192.168.1.10",
			wantName:    "192.168.1.10",
			wantKind:    "host",
		},
		{
			name: "map with name and address and tags",
			input: map[string]any{
				"name":    "web-prod-1",
				"address": "10.0.0.1",
				"kind":    "vm",
				"role":    "web",
				"env":     "prod",
			},
			wantAddress: "10.0.0.1",
			wantName:    "web-prod-1",
			wantKind:    "vm",
			wantRole:    "web",
		},
		{
			name: "map with host instead of address",
			input: map[string]any{
				"host": "web-2.corp.local",
				"role": "web",
			},
			wantAddress: "web-2.corp.local",
			wantName:    "web-2.corp.local",
			wantKind:    "host",
			wantRole:    "web",
		},
		{
			name:    "empty string error",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "empty map error",
			input:   map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NormalizeResource(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeResource() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if r.Address != tt.wantAddress {
				t.Errorf("Address = %q, want %q", r.Address, tt.wantAddress)
			}
			if r.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", r.Name, tt.wantName)
			}
			if r.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", r.Kind, tt.wantKind)
			}
			if tt.wantRole != "" && r.Labels["role"] != tt.wantRole {
				t.Errorf("Labels[role] = %q, want %q", r.Labels["role"], tt.wantRole)
			}
		})
	}
}

func TestFleetFiltering(t *testing.T) {
	resources := []Resource{
		{ID: "1", Name: "web-1", Address: "10.0.1.1", Kind: "host", Labels: map[string]string{"role": "web", "env": "prod"}},
		{ID: "2", Name: "web-2", Address: "10.0.1.2", Kind: "host", Labels: map[string]string{"role": "web", "env": "stage"}},
		{ID: "3", Name: "db-1", Address: "10.0.2.1", Kind: "host", Labels: map[string]string{"role": "db", "env": "prod"}},
	}
	f := New(resources)

	// Filter by single exact keyword
	res, err := f.filterBuiltin(nil, nil, nil, []starlark.Tuple{
		{starlark.String("role"), starlark.String("web")},
	})
	if err != nil {
		t.Fatal(err)
	}
	filteredFleet := res.(*Fleet)
	if len(filteredFleet.resources) != 2 {
		t.Fatalf("expected 2 web resources, got %d", len(filteredFleet.resources))
	}

	// Filter by multiple keywords
	res, err = f.filterBuiltin(nil, nil, nil, []starlark.Tuple{
		{starlark.String("role"), starlark.String("web")},
		{starlark.String("env"), starlark.String("prod")},
	})
	if err != nil {
		t.Fatal(err)
	}
	filteredFleet = res.(*Fleet)
	if len(filteredFleet.resources) != 1 || filteredFleet.resources[0].Name != "web-1" {
		t.Fatalf("expected web-1, got %v", filteredFleet.resources)
	}

	// Filter by predicate function in Starlark thread
	thread := &starlark.Thread{Name: "test-filter"}
	predBuiltin := starlark.NewBuiltin("pred", func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		dict := args[0].(*starlark.Dict)
		v, found, _ := dict.Get(starlark.String("role"))
		if found && v.(starlark.String) == "db" {
			return starlark.True, nil
		}
		return starlark.False, nil
	})

	res, err = f.filterBuiltin(thread, nil, starlark.Tuple{predBuiltin}, nil)
	if err != nil {
		t.Fatal(err)
	}
	filteredFleet = res.(*Fleet)
	if len(filteredFleet.resources) != 1 || filteredFleet.resources[0].Name != "db-1" {
		t.Fatalf("expected db-1, got %v", filteredFleet.resources)
	}
}

func TestFleetGroupBy(t *testing.T) {
	resources := []Resource{
		{ID: "1", Name: "web-1", Address: "10.0.1.1", Labels: map[string]string{"role": "web"}},
		{ID: "2", Name: "web-2", Address: "10.0.1.2", Labels: map[string]string{"role": "web"}},
		{ID: "3", Name: "db-1", Address: "10.0.2.1", Labels: map[string]string{"role": "db"}},
	}
	f := New(resources)

	res, err := f.groupByBuiltin(nil, starlark.NewBuiltin("group_by", nil), starlark.Tuple{starlark.String("role")}, nil)
	if err != nil {
		t.Fatal(err)
	}

	dict := res.(*starlark.Dict)
	if dict.Len() != 2 {
		t.Fatalf("expected 2 groups, got %d", dict.Len())
	}

	webGroupVal, found, _ := dict.Get(starlark.String("web"))
	if !found {
		t.Fatal("expected 'web' group")
	}
	webFleet := webGroupVal.(*Fleet)
	if len(webFleet.resources) != 2 {
		t.Fatalf("expected 2 web resources in group, got %d", len(webFleet.resources))
	}
}

func TestFleetExtractionMethods(t *testing.T) {
	resources := []Resource{
		{ID: "id-1", Name: "web-1", Address: "10.0.1.1", Labels: map[string]string{"custom_ip": "172.16.0.1"}},
		{ID: "id-2", Name: "web-2", Address: "10.0.1.2", Labels: map[string]string{"custom_ip": "172.16.0.2"}},
	}
	f := New(resources)

	// addresses()
	addrsVal, err := f.addressesBuiltin(nil, starlark.NewBuiltin("addresses", nil), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	addrsList := addrsVal.(*starlark.List)
	if addrsList.Len() != 2 || addrsList.Index(0).(starlark.String) != "10.0.1.1" {
		t.Errorf("unexpected addresses: %v", addrsList)
	}

	// addresses(key="custom_ip")
	customAddrsVal, err := f.addressesBuiltin(nil, starlark.NewBuiltin("addresses", nil), nil, []starlark.Tuple{
		{starlark.String("key"), starlark.String("custom_ip")},
	})
	if err != nil {
		t.Fatal(err)
	}
	customList := customAddrsVal.(*starlark.List)
	if customList.Len() != 2 || customList.Index(0).(starlark.String) != "172.16.0.1" {
		t.Errorf("unexpected custom addresses: %v", customList)
	}

	// names()
	namesVal, err := f.namesBuiltin(nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	namesList := namesVal.(*starlark.List)
	if namesList.Len() != 2 || namesList.Index(0).(starlark.String) != "web-1" {
		t.Errorf("unexpected names: %v", namesList)
	}

	// ids()
	idsVal, err := f.idsBuiltin(nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	idsList := idsVal.(*starlark.List)
	if idsList.Len() != 2 || idsList.Index(0).(starlark.String) != "id-1" {
		t.Errorf("unexpected ids: %v", idsList)
	}

	// first()
	firstVal, err := f.firstBuiltin(nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstDict := firstVal.(*starlark.Dict)
	nameVal, _, _ := firstDict.Get(starlark.String("name"))
	if nameVal.(starlark.String) != "web-1" {
		t.Errorf("unexpected first name: %v", nameVal)
	}
}

func TestFleetFromSource(t *testing.T) {
	thread := &starlark.Thread{Name: "test-from-source"}

	// 1. From List of dicts
	d1 := starlark.NewDict(2)
	d1.SetKey(starlark.String("name"), starlark.String("h1"))
	d1.SetKey(starlark.String("address"), starlark.String("1.1.1.1"))

	d2 := starlark.NewDict(2)
	d2.SetKey(starlark.String("name"), starlark.String("h2"))
	d2.SetKey(starlark.String("address"), starlark.String("2.2.2.2"))

	itemsList := starlark.NewList([]starlark.Value{d1, d2})
	f1, err := FromSource(thread, itemsList)
	if err != nil {
		t.Fatal(err)
	}
	if len(f1.resources) != 2 || f1.resources[0].Name != "h1" {
		t.Fatalf("unexpected list parse result: %v", f1.resources)
	}

	// 2. From Callable function
	callFn := starlark.NewBuiltin("discover", func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.NewList([]starlark.Value{
			starlark.String("10.0.0.1"),
			starlark.String("10.0.0.2"),
		}), nil
	})
	f2, err := FromSource(thread, callFn)
	if err != nil {
		t.Fatal(err)
	}
	if len(f2.resources) != 2 || f2.resources[0].Address != "10.0.0.1" {
		t.Fatalf("unexpected callable parse result: %v", f2.resources)
	}

	// 3. From JSON string
	jsonStr := starlark.String(`[{"name": "json-node", "address": "172.16.1.10", "role": "cache"}]`)
	f3, err := FromSource(thread, jsonStr)
	if err != nil {
		t.Fatal(err)
	}
	if len(f3.resources) != 1 || f3.resources[0].Name != "json-node" || f3.resources[0].Labels["role"] != "cache" {
		t.Fatalf("unexpected json string parse result: %v", f3.resources)
	}
}

func TestFleetEqualityAndCloning(t *testing.T) {
	origResources := []Resource{
		{Name: "n1", Address: "1.1.1.1"},
	}
	f := New(origResources)
	origResources[0].Name = "mutated"

	if f.resources[0].Name != "n1" {
		t.Fatalf("Fleet must defensively copy resources slice on creation")
	}

	retrieved := f.Resources()
	retrieved[0].Name = "mutated2"
	if f.resources[0].Name != "n1" {
		t.Fatalf("Fleet.Resources() must return defensive copy")
	}

	if !reflect.DeepEqual(f.Resources(), []Resource{{Name: "n1", Address: "1.1.1.1"}}) {
		t.Fatalf("unexpected resources slice")
	}
}

func TestFleetStringFormat(t *testing.T) {
	f := New([]Resource{{Name: "n1", Address: "1.1.1.1"}})
	if !strings.Contains(f.String(), "count=1") {
		t.Errorf("unexpected string: %s", f.String())
	}
}
