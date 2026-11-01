package catalog

import (
	"maps"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/catalog/apidocs"
)

var defaultFactoryReturns = map[string]string{
	// Filesystem
	"fs.path": "fs.path",

	// HTTP
	"http.url":         "http.url",
	"http.server":      "http.server",
	"http.get":         "http.response",
	"http.post":        "http.response",
	"http.put":         "http.response",
	"http.patch":       "http.response",
	"http.delete":      "http.response",
	"http.head":        "http.response",
	"http.options":     "http.response",
	"http.url.get":     "http.response",
	"http.url.post":    "http.response",
	"http.url.put":     "http.response",
	"http.url.patch":   "http.response",
	"http.url.delete":  "http.response",
	"http.url.head":    "http.response",
	"http.url.options": "http.response",

	// Structured Data
	"json.file": "json.file",
	"yaml.file": "yaml.file",
	"csv.file":  "csv.file",
	"zip.file":  "zip.file",
	"gzip.file": "gzip.file",

	// Cloud, Database & Remote Ops
	"k8s.cluster":             "k8s.client",
	"k8s.client":              "k8s.client",
	"sql.connect":             "sql.connection",
	"sql.open":                "sql.connection",
	"sql.connection.begin":    "sql.transaction",
	"sql.connection.prepare":  "sql.stmt",
	"sql.transaction.prepare": "sql.stmt",
	"ssh.client":              "ssh.client",
	"ssh.connect":             "ssh.client",
	"ssh.config":              "ssh.client",
	"containers.client":       "containers.client",
}

// Catalog represents the unified metadata catalog for a Starkite runtime edition.
type Catalog struct {
	globals        []Symbol
	members        map[string][]Symbol
	factoryReturns map[string]string
	modules        map[string]string
	docs           *DocIndex
}

// Build constructs a Catalog from a module registry and static API doc tables.
func Build(reg *libkite.Registry) *Catalog {
	c := &Catalog{
		members:        make(map[string][]Symbol),
		factoryReturns: make(map[string]string),
		modules:        make(map[string]string),
		docs:           NewDocIndex(apidocs.Table),
	}

	maps.Copy(c.factoryReturns, defaultFactoryReturns)

	loaded, _ := reg.LoadAll()

	// Pass 1: Module namespaces, members, and declarative types
	for _, name := range reg.Names() {
		moduleName := string(name)
		description := ""
		m, ok := reg.Get(name)
		if ok {
			description = m.Description()
		}
		c.modules[moduleName] = description
		c.globals = append(c.globals, Symbol{
			Name: moduleName,
			Kind: KindNamespace,
			Doc:  description,
		})

		// Introspect module namespace members
		if value, ok := loaded[moduleName]; ok {
			c.members[moduleName] = c.attrSymbols(value, moduleName)
		}

		// Introspect declarative types if module implements TypedModule
		if tm, ok := m.(libkite.TypedModule); ok {
			for _, td := range tm.ExportedTypes() {
				if td.Value != nil {
					c.members[td.Name] = c.attrSymbols(td.Value, td.Name)
				}
				for method, retType := range td.Returns {
					c.factoryReturns[td.Name+"."+method] = retType
				}
			}
		}
	}

	// Pass 2: Global aliases and predeclared identifiers
	seen := make(map[string]bool, len(c.globals))
	for _, g := range c.globals {
		seen[g.Name] = true
	}
	addGlobal := func(name string, value starlark.Value) {
		if seen[name] {
			return
		}
		seen[name] = true
		c.globals = append(c.globals, c.globalSymbol(name, value))
	}
	for name, value := range reg.GetAliases() {
		addGlobal(name, value)
	}
	for name, value := range reg.Predeclared() {
		addGlobal(name, value)
	}

	sort.Slice(c.globals, func(i, j int) bool { return c.globals[i].Name < c.globals[j].Name })
	return c
}

func (c *Catalog) attrSymbols(val starlark.Value, owner string) []Symbol {
	hasAttrs, ok := val.(starlark.HasAttrs)
	if !ok {
		return nil
	}
	attrNames := hasAttrs.AttrNames()
	out := make([]Symbol, 0, len(attrNames))
	seen := make(map[string]bool, len(attrNames))

	for _, name := range attrNames {
		if seen[name] {
			continue
		}
		seen[name] = true

		sym := Symbol{
			Name:  name,
			Owner: owner,
			Kind:  KindFunction,
		}
		if doc, found := c.docs.LookupMember(owner, name); found {
			sym.Doc = doc.Doc
			sym.Signature = doc.Signature
			sym.Params = doc.Params
			sym.Returns = doc.Returns
		}

		// Sort try_ variants after plain variants
		if strings.HasPrefix(name, "try_") {
			sym.SortText = "9" + name
		} else {
			sym.SortText = "0" + name
		}

		out = append(out, sym)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *Catalog) globalSymbol(name string, val starlark.Value) Symbol {
	sym := Symbol{
		Name: name,
		Kind: KindVariable,
	}
	if _, ok := val.(*starlark.Builtin); ok {
		sym.Kind = KindFunction
	}
	if doc, found := c.docs.LookupGlobal(name); found {
		sym.Doc = doc.Doc
		sym.Signature = doc.Signature
		sym.Params = doc.Params
		sym.Returns = doc.Returns
	}
	return sym
}

// Globals returns all top-level symbols (namespaces, aliases, predeclared).
func (c *Catalog) Globals() []Symbol {
	return c.globals
}

// Members returns symbols exposed by an owner (module name or object type).
func (c *Catalog) Members(owner string) []Symbol {
	return c.members[owner]
}

// IsModule reports whether name is a registered module namespace.
func (c *Catalog) IsModule(name string) bool {
	_, ok := c.modules[name]
	return ok
}

// LookupModule returns the description for a module namespace.
func (c *Catalog) LookupModule(name string) (string, bool) {
	desc, ok := c.modules[name]
	return desc, ok
}

// FactoryReturns returns the object type returned by a callee expression.
func (c *Catalog) FactoryReturns(callee string) (string, bool) {
	typ, ok := c.factoryReturns[callee]
	return typ, ok
}

// LookupGlobal finds documentation for a top-level global name.
func (c *Catalog) LookupGlobal(name string) (Symbol, bool) {
	for _, g := range c.globals {
		if g.Name == name {
			return g, true
		}
	}
	return Symbol{}, false
}

// LookupMember finds documentation for a member on an owner.
func (c *Catalog) LookupMember(owner, name string) (Symbol, bool) {
	for _, m := range c.members[owner] {
		if m.Name == name {
			return m, true
		}
	}
	// Fall back to doc index if member is not directly on prototype
	if doc, found := c.docs.LookupMember(owner, name); found {
		return Symbol{
			Name:      name,
			Owner:     owner,
			Kind:      KindFunction,
			Doc:       doc.Doc,
			Signature: doc.Signature,
			Params:    doc.Params,
			Returns:   doc.Returns,
		}, true
	}
	return Symbol{}, false
}

// Modules returns all registered module names and descriptions.
func (c *Catalog) Modules() map[string]string {
	out := make(map[string]string, len(c.modules))
	maps.Copy(out, c.modules)
	return out
}
