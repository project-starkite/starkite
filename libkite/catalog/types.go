package catalog

// Kind categorizes a symbol in the Starkite completion and documentation catalog.
type Kind int

const (
	KindUnknown Kind = iota
	KindNamespace
	KindFunction
	KindVariable
	KindProperty
)

func (k Kind) String() string {
	switch k {
	case KindNamespace:
		return "namespace"
	case KindFunction:
		return "function"
	case KindVariable:
		return "variable"
	case KindProperty:
		return "property"
	default:
		return "unknown"
	}
}

// Symbol represents an inspectable, completable, and documentable Starkite entity.
type Symbol struct {
	Name      string   // Identifier name (e.g. "path", "read_text", "env")
	Owner     string   // Namespace or object type owner (e.g. "fs", "fs.path", "os")
	Kind      Kind     // Namespace, Function, Variable, or Property
	Doc       string   // Human-readable documentation prose
	Signature string   // Call signature (e.g. "fs.path(path)")
	Params    []string // Parameter names parsed from documentation
	Returns   string   // Return type name (e.g. "Path", "Result", "string")
	SortText  string   // Ordering hint for editor completion
}
