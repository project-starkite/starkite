package lsp

import (
	"strings"

	"github.com/project-starkite/starkite/libkite/lsp/apidocs"
)

// docEntry is what hover and signature help need about one name.
type docEntry struct {
	signature string
	params    []string
	returns   string
	doc       string
}

// docIndex resolves a name to its documented signature and prose.
//
// The runtime registry is authoritative about which names exist. This index is
// authoritative about what they take and what they mean, because a Starlark
// builtin carries neither its parameter names nor its description.
//
// The two sources do not share a join key. The registry addresses a method by
// the object type that holds it — "fs.path" — while the documents address it
// by a receiver variable — "p.read_text()". The rule that bridges them: inside
// one module's reference file, a row whose receiver equals the module name is a
// module-level function, and a row with any other receiver is a method on that
// module's object.
type docIndex struct {
	// byModule maps a module name to the module-level entries in its file.
	byModule map[string]map[string]docEntry

	// byObject maps a module name to the object-method entries in its file,
	// keyed by member name. Modules that document more than one object type
	// share this map; see the note in lookupMember.
	byObject map[string]map[string]docEntry

	// globals maps a bare documented name to its entry.
	globals map[string]docEntry

	// globalsByModule keeps the same bare entries under the reference file
	// they came from. A module function is often documented only as the
	// global alias it forwards to — fs.md writes `path(p)`, not `fs.path(p)`
	// — so a module lookup has to be able to find it.
	globalsByModule map[string]map[string]docEntry
}

func newDocIndex(entries []apidocs.Entry) *docIndex {
	idx := &docIndex{
		byModule:        make(map[string]map[string]docEntry),
		byObject:        make(map[string]map[string]docEntry),
		globals:         make(map[string]docEntry),
		globalsByModule: make(map[string]map[string]docEntry),
	}
	for _, e := range entries {
		entry := docEntry{
			signature: e.Signature,
			params:    e.Params,
			returns:   e.Returns,
			doc:       e.Doc,
		}
		switch {
		case e.Receiver == "":
			// A bare name under a Method or Function heading is a global
			// alias the module exports at the top level.
			if _, exists := idx.globals[e.Name]; !exists {
				idx.globals[e.Name] = entry
			}
			put(idx.globalsByModule, e.Module, e.Name, entry)
		case e.Receiver == e.Module:
			put(idx.byModule, e.Module, e.Name, entry)
		default:
			// Both a named receiver ("p", "srv") and the ObjectReceiver
			// marker mean the member hangs off the module's object.
			put(idx.byObject, e.Module, e.Name, entry)
		}
	}
	return idx
}

func put(m map[string]map[string]docEntry, outer, inner string, e docEntry) {
	bucket, ok := m[outer]
	if !ok {
		bucket = make(map[string]docEntry)
		m[outer] = bucket
	}
	if _, exists := bucket[inner]; !exists {
		bucket[inner] = e
	}
}

// lookupGlobal finds documentation for a top-level name.
func (idx *docIndex) lookupGlobal(name string) (docEntry, bool) {
	if idx == nil {
		return docEntry{}, false
	}
	if e, ok := idx.globals[name]; ok {
		return e, true
	}
	// A global alias is usually documented as the module function it forwards
	// to — `read_text` as `p.read_text()` in the fs reference. Accept a unique
	// match on the bare name across every module's object tables.
	var found docEntry
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
	return docEntry{}, false
}

// lookupMember finds documentation for a name reached through a dot.
//
// owner is either a module name ("fs") or an object type as the runtime
// reports it ("fs.path", "base64.source"). For an object type the module is
// the segment before the first dot.
//
// A module that documents more than one object type — http documents both the
// request builder and the response — shares one object bucket, so a member name
// used by two of its types resolves to whichever the file lists first. That is
// a documentation-shape limitation rather than a runtime one; the name set
// stays correct because it comes from the registry.
func (idx *docIndex) lookupMember(owner, name string) (docEntry, bool) {
	if idx == nil {
		return docEntry{}, false
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
	// The reference file documented it as a bare global alias.
	if bucket, ok := idx.globalsByModule[module]; ok {
		if e, ok := bucket[name]; ok {
			return e, true
		}
	}

	// A try_ variant inherits the documentation of the call it wraps, with a
	// Result return type.
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if e, found := idx.lookupMember(owner, base); found {
			e.returns = "Result"
			e.doc = e.doc + " Returns a Result instead of raising."
			return e, true
		}
	}
	return docEntry{}, false
}
