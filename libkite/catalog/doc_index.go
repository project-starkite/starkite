package catalog

import (
	"strings"

	"github.com/project-starkite/starkite/libkite/catalog/apidocs"
)

// DocEntry represents documentation metadata for a symbol.
type DocEntry struct {
	Signature string
	Params    []string
	Returns   string
	Doc       string
}

// DocIndex resolves names to their documented signatures and descriptions.
//
// The runtime registry is authoritative about which names exist. This index is
// authoritative about what they take and what they mean, because a Starlark
// builtin carries neither its parameter names nor its description.
type DocIndex struct {
	byModule        map[string]map[string]DocEntry
	byObject        map[string]map[string]DocEntry
	globals         map[string]DocEntry
	globalsByModule map[string]map[string]DocEntry
}

// NewDocIndex builds a DocIndex from the compiled API doc table entries.
func NewDocIndex(entries []apidocs.Entry) *DocIndex {
	idx := &DocIndex{
		byModule:        make(map[string]map[string]DocEntry),
		byObject:        make(map[string]map[string]DocEntry),
		globals:         make(map[string]DocEntry),
		globalsByModule: make(map[string]map[string]DocEntry),
	}
	for _, e := range entries {
		entry := DocEntry{
			Signature: e.Signature,
			Params:    e.Params,
			Returns:   e.Returns,
			Doc:       e.Doc,
		}
		switch {
		case e.Receiver == "":
			if _, exists := idx.globals[e.Name]; !exists {
				idx.globals[e.Name] = entry
			}
			putDoc(idx.globalsByModule, e.Module, e.Name, entry)
		case e.Receiver == e.Module:
			putDoc(idx.byModule, e.Module, e.Name, entry)
		default:
			putDoc(idx.byObject, e.Module, e.Name, entry)
		}
	}
	return idx
}

func putDoc(m map[string]map[string]DocEntry, outer, inner string, e DocEntry) {
	bucket, ok := m[outer]
	if !ok {
		bucket = make(map[string]DocEntry)
		m[outer] = bucket
	}
	if _, exists := bucket[inner]; !exists {
		bucket[inner] = e
	}
}

// LookupGlobal finds documentation for a top-level name.
func (idx *DocIndex) LookupGlobal(name string) (DocEntry, bool) {
	if idx == nil {
		return DocEntry{}, false
	}
	if e, ok := idx.globals[name]; ok {
		return e, true
	}
	var found DocEntry
	hits := 0
	for _, bucket := range idx.byObject {
		if e, ok := bucket[name]; ok {
			found, hits = e, hits+1
		}
	}
	for _, bucket := range idx.byModule {
		if e, ok := bucket[name]; ok {
			found, hits = e, hits+1
		}
	}
	if hits == 1 {
		return found, true
	}
	return DocEntry{}, false
}

// LookupMember finds documentation for a name reached through a dot.
func (idx *DocIndex) LookupMember(owner, name string) (DocEntry, bool) {
	if idx == nil {
		return DocEntry{}, false
	}
	if bucket, ok := idx.byModule[owner]; ok {
		if e, ok := bucket[name]; ok {
			return e, true
		}
	}

	module := owner
	if dot := strings.Index(owner, "."); dot >= 0 {
		module = owner[:dot]
	}
	if bucket, ok := idx.byObject[module]; ok {
		if e, ok := bucket[name]; ok {
			return e, true
		}
	}
	if bucket, ok := idx.byModule[module]; ok {
		if e, ok := bucket[name]; ok {
			return e, true
		}
	}
	if bucket, ok := idx.globalsByModule[module]; ok {
		if e, ok := bucket[name]; ok {
			return e, true
		}
	}

	// try_ variant inherits the documentation of the call it wraps, with Result return type.
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if e, found := idx.LookupMember(owner, base); found {
			e.Returns = "Result"
			if e.Doc != "" {
				e.Doc = e.Doc + " Returns a Result instead of raising."
			} else {
				e.Doc = "Returns a Result instead of raising."
			}
			return e, true
		}
	}
	return DocEntry{}, false
}
