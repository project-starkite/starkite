package lsp

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/M31-Labs/starlsp"
	"go.starlark.net/syntax"
)

const (
	structOwnerPrefix   = "struct:"
	maxStructSchemas    = 512
	maxCachedStarFiles  = 128
	maxInferExprDepth   = 48
	maxInferLoadDepth   = 6
	maxTolerantParseFix = 8
)

type structMember struct {
	symbol      starlsp.Symbol
	valueOwner  string
	returnOwner string
}

type structSchema struct {
	owner   string
	members []starlsp.Symbol
	byName  map[string]structMember
}

type cachedStarFile struct {
	modTime int64
	size    int64
	mod     *parsedModule
}

type parsedModule struct {
	path     string
	assigns  map[string]syntax.Expr
	fallback map[string]syntax.Expr
	defs     map[string]*syntax.DefStmt
	loads    map[string]loadBinding
}

type loadBinding struct {
	target string
	export string
}

type inferredValue struct {
	kind        starlsp.Kind
	valueOwner  string
	returnOwner string
	signature   string
	returnsText string
	doc         string
	defStmt     *syntax.DefStmt
}

type inferContext struct {
	mod         *parsedModule
	locals      map[string]syntax.Expr
	useFallback bool
	exprDepth   int
	loadDepth   int
	visited     map[string]bool
	resolveCB   func(string) (string, bool)
}

func (h *Host) structMembers(owner string) []starlsp.Symbol {
	h.mu.RLock()
	schema := h.structs[owner]
	h.mu.RUnlock()
	if schema == nil || len(schema.members) == 0 {
		return nil
	}
	out := make([]starlsp.Symbol, len(schema.members))
	copy(out, schema.members)
	return out
}

func (h *Host) structDocument(owner, name string) (starlsp.Symbol, bool) {
	h.mu.RLock()
	schema := h.structs[owner]
	if schema == nil {
		h.mu.RUnlock()
		return starlsp.Symbol{}, false
	}
	m, ok := schema.byName[name]
	h.mu.RUnlock()
	if !ok {
		return starlsp.Symbol{}, false
	}
	return m.symbol, true
}

func (h *Host) lookupStructSchema(owner string) *structSchema {
	if !strings.HasPrefix(owner, structOwnerPrefix) {
		return nil
	}
	h.mu.RLock()
	schema := h.structs[owner]
	h.mu.RUnlock()
	return schema
}

func (h *Host) putStructSchema(members []structMember) string {
	slices.SortFunc(members, func(a, b structMember) int {
		return cmp.Compare(a.symbol.Name, b.symbol.Name)
	})

	sum := sha256.New()
	for _, m := range members {
		_, _ = sum.Write([]byte(m.symbol.Name))
		_, _ = sum.Write([]byte{0})
		_, _ = sum.Write([]byte{byte(m.symbol.Kind)})
		_, _ = sum.Write([]byte{0})
		_, _ = sum.Write([]byte(m.symbol.Signature))
		_, _ = sum.Write([]byte{0})
		_, _ = sum.Write([]byte(m.symbol.Returns))
		_, _ = sum.Write([]byte{0})
		_, _ = sum.Write([]byte(m.valueOwner))
		_, _ = sum.Write([]byte{0})
		_, _ = sum.Write([]byte(m.returnOwner))
		_, _ = sum.Write([]byte{'\n'})
	}
	owner := structOwnerPrefix + hex.EncodeToString(sum.Sum(nil))[:16]

	syms := make([]starlsp.Symbol, len(members))
	byName := make(map[string]structMember, len(members))
	for i := range members {
		members[i].symbol.Owner = owner
		syms[i] = members[i].symbol
		byName[members[i].symbol.Name] = members[i]
	}
	schema := &structSchema{
		owner:   owner,
		members: syms,
		byName:  byName,
	}

	h.mu.Lock()
	if h.structs == nil {
		h.structs = make(map[string]*structSchema, 32)
	}
	if _, exists := h.structs[owner]; !exists && len(h.structs) >= maxStructSchemas {
		clear(h.structs)
	}
	h.structs[owner] = schema
	h.mu.Unlock()

	return owner
}

func (h *Host) resolveStructType(req starlsp.TypeRequest) (string, bool) {
	expr, err := (&syntax.FileOptions{}).ParseExpr("", req.Expr, 0)
	if err != nil {
		return "", false
	}

	var docMod *parsedModule
	if req.Document != nil {
		docPath := req.Document.Path()
		file, _ := parseTolerant(docPath, req.Document.Text())
		if file != nil {
			docMod = buildParsedModule(docPath, file, true)
		}
	}

	ctx := &inferContext{
		mod:         docMod,
		useFallback: docMod != nil,
		visited:     make(map[string]bool),
		resolveCB:   req.Resolve,
	}
	val := h.inferExpr(expr, ctx)
	if val.valueOwner != "" {
		return val.valueOwner, true
	}
	return "", false
}

func parseTolerant(path string, src []byte) (*syntax.File, bool) {
	opts := &syntax.FileOptions{TopLevelControl: true, GlobalReassign: true, Set: true, While: true}
	if f, err := opts.Parse(path, src, 0); err == nil {
		return f, true
	}

	lines := strings.Split(string(src), "\n")
	patched := make(map[int]bool)

	for range maxTolerantParseFix {
		f, err := opts.Parse(path, strings.Join(lines, "\n"), 0)
		if err == nil {
			return f, true
		}
		synErr, ok := err.(syntax.Error)
		if !ok {
			break
		}
		idx := max(int(synErr.Pos.Line)-1, 0)
		if idx >= len(lines) {
			idx = len(lines) - 1
		}
		// Step backward past blank or already-patched lines (common on trailing dots at EOF).
		for idx >= 0 && (strings.TrimSpace(lines[idx]) == "" || patched[idx]) {
			idx--
		}
		if idx < 0 {
			break
		}
		indentLen := len(lines[idx]) - len(strings.TrimLeft(lines[idx], " \t"))
		if indentLen > 0 {
			lines[idx] = lines[idx][:indentLen] + "pass"
		} else {
			lines[idx] = ""
		}
		patched[idx] = true
	}
	return nil, false
}

func buildParsedModule(path string, file *syntax.File, collectFallback bool) *parsedModule {
	mod := &parsedModule{
		path:    path,
		assigns: make(map[string]syntax.Expr),
		defs:    make(map[string]*syntax.DefStmt),
		loads:   make(map[string]loadBinding),
	}
	if collectFallback {
		mod.fallback = make(map[string]syntax.Expr)
	}

	for _, stmt := range file.Stmts {
		switch s := stmt.(type) {
		case *syntax.LoadStmt:
			target := s.ModuleName()
			for i, toIdent := range s.To {
				if toIdent == nil {
					continue
				}
				exportName := toIdent.Name
				if i < len(s.From) && s.From[i] != nil {
					exportName = s.From[i].Name
				}
				mod.loads[toIdent.Name] = loadBinding{target: target, export: exportName}
			}
		case *syntax.DefStmt:
			if s.Name != nil {
				mod.defs[s.Name.Name] = s
			}
			if collectFallback {
				collectInnerAssigns(s.Body, mod.fallback)
			}
		case *syntax.AssignStmt:
			recordAssign(s, mod.assigns)
		default:
			if collectFallback {
				collectStmtAssigns(s, mod.fallback)
			}
		}
	}
	return mod
}

func recordAssign(s *syntax.AssignStmt, dst map[string]syntax.Expr) {
	if s.Op != syntax.EQ {
		return
	}
	id, ok := s.LHS.(*syntax.Ident)
	if !ok || id == nil || s.RHS == nil {
		return
	}
	dst[id.Name] = s.RHS
}

func collectInnerAssigns(stmts []syntax.Stmt, dst map[string]syntax.Expr) {
	for _, stmt := range stmts {
		collectStmtAssigns(stmt, dst)
	}
}

func collectStmtAssigns(stmt syntax.Stmt, dst map[string]syntax.Expr) {
	switch s := stmt.(type) {
	case *syntax.AssignStmt:
		recordAssign(s, dst)
	case *syntax.IfStmt:
		collectInnerAssigns(s.True, dst)
		collectInnerAssigns(s.False, dst)
	case *syntax.ForStmt:
		collectInnerAssigns(s.Body, dst)
	case *syntax.WhileStmt:
		collectInnerAssigns(s.Body, dst)
	}
}

func (h *Host) loadModule(path string) *parsedModule {
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil || !info.Mode().IsRegular() || info.Size() > DefaultMaxContentLength {
		return nil
	}
	modTime := info.ModTime().UnixNano()
	size := info.Size()

	h.mu.RLock()
	cached, ok := h.fileCache[clean]
	h.mu.RUnlock()
	if ok && cached.modTime == modTime && cached.size == size {
		return cached.mod
	}

	data, err := os.ReadFile(clean)
	if err != nil {
		return nil
	}
	file, _ := parseTolerant(clean, data)
	if file == nil {
		return nil
	}
	mod := buildParsedModule(clean, file, false)

	h.mu.Lock()
	if h.fileCache == nil {
		h.fileCache = make(map[string]cachedStarFile, 16)
	}
	if _, exists := h.fileCache[clean]; !exists && len(h.fileCache) >= maxCachedStarFiles {
		clear(h.fileCache)
	}
	h.fileCache[clean] = cachedStarFile{
		modTime: modTime,
		size:    size,
		mod:     mod,
	}
	h.mu.Unlock()

	return mod
}

func (ctx *inferContext) childExpr() *inferContext {
	return &inferContext{
		mod:         ctx.mod,
		locals:      ctx.locals,
		useFallback: ctx.useFallback,
		exprDepth:   ctx.exprDepth + 1,
		loadDepth:   ctx.loadDepth,
		visited:     ctx.visited,
		resolveCB:   ctx.resolveCB,
	}
}

func (h *Host) inferExpr(expr syntax.Expr, ctx *inferContext) inferredValue {
	if expr == nil || ctx.exprDepth >= maxInferExprDepth {
		return inferredValue{}
	}

	switch e := expr.(type) {
	case *syntax.ParenExpr:
		return h.inferExpr(e.X, ctx.childExpr())

	case *syntax.Literal:
		return inferredValue{kind: starlsp.KindProperty, returnsText: literalTypeName(e)}

	case *syntax.ListExpr, *syntax.Comprehension:
		if comp, ok := e.(*syntax.Comprehension); ok && comp.Curly {
			return inferredValue{kind: starlsp.KindProperty, returnsText: "dict"}
		}
		return inferredValue{kind: starlsp.KindProperty, returnsText: "list"}

	case *syntax.DictExpr:
		return inferredValue{kind: starlsp.KindProperty, returnsText: "dict"}

	case *syntax.UnaryExpr:
		return h.inferExpr(e.X, ctx.childExpr())

	case *syntax.CondExpr:
		tVal := h.inferExpr(e.True, ctx.childExpr())
		if tVal.valueOwner != "" {
			return tVal
		}
		return h.inferExpr(e.False, ctx.childExpr())

	case *syntax.BinaryExpr:
		if e.Op == syntax.PLUS {
			return h.inferStructMerge(e.X, e.Y, ctx)
		}
		return inferredValue{kind: starlsp.KindProperty}

	case *syntax.Ident:
		return h.inferIdent(e.Name, ctx)

	case *syntax.DotExpr:
		return h.inferDot(e, ctx)

	case *syntax.CallExpr:
		return h.inferCall(e, ctx)
	}

	return inferredValue{kind: starlsp.KindProperty}
}

func (h *Host) mergeStructSchemas(leftOwner, rightOwner string) string {
	ls := h.lookupStructSchema(leftOwner)
	rs := h.lookupStructSchema(rightOwner)
	switch {
	case ls == nil && rs == nil:
		return ""
	case ls == nil:
		return rightOwner
	case rs == nil:
		return leftOwner
	}

	merged := make(map[string]structMember, len(ls.byName)+len(rs.byName))
	maps.Copy(merged, ls.byName)
	maps.Copy(merged, rs.byName)
	members := make([]structMember, 0, len(merged))
	for _, m := range merged {
		members = append(members, m)
	}
	return h.putStructSchema(members)
}

func (h *Host) inferStructMerge(leftExpr, rightExpr syntax.Expr, ctx *inferContext) inferredValue {
	child := ctx.childExpr()
	left := h.inferExpr(leftExpr, child)
	right := h.inferExpr(rightExpr, child)

	owner := h.mergeStructSchemas(left.valueOwner, right.valueOwner)
	if owner == "" {
		return inferredValue{kind: starlsp.KindProperty}
	}
	return inferredValue{kind: starlsp.KindProperty, valueOwner: owner, returnsText: "struct"}
}

func (h *Host) inferIdent(name string, ctx *inferContext) inferredValue {
	if name == "" {
		return inferredValue{}
	}
	if h.cat != nil && h.cat.IsModule(name) {
		return inferredValue{kind: starlsp.KindProperty, valueOwner: name, returnsText: name}
	}
	if ctx.mod == nil && len(ctx.locals) == 0 {
		return inferredValue{}
	}

	modPath := ""
	if ctx.mod != nil {
		modPath = ctx.mod.path
	}
	visitKey := "id:" + modPath + ":" + name
	if ctx.locals != nil {
		visitKey = "local:" + modPath + ":" + name
	}
	if ctx.visited[visitKey] {
		return inferredValue{}
	}
	ctx.visited[visitKey] = true
	defer delete(ctx.visited, visitKey)

	// Function-local variables take precedence over module scope.
	if ctx.locals != nil {
		if rhs, ok := ctx.locals[name]; ok {
			return h.inferExpr(rhs, ctx.childExpr())
		}
	}
	if ctx.mod == nil {
		return inferredValue{}
	}

	if defStmt, ok := ctx.mod.defs[name]; ok {
		return h.inferDefSymbol(name, defStmt, &inferContext{
			mod:       ctx.mod,
			exprDepth: ctx.exprDepth + 1,
			loadDepth: ctx.loadDepth,
			visited:   ctx.visited,
			resolveCB: ctx.resolveCB,
		})
	}

	if lb, ok := ctx.mod.loads[name]; ok {
		return h.inferLoadedSymbol(lb, ctx)
	}

	if rhs, ok := ctx.mod.assigns[name]; ok {
		return h.inferExpr(rhs, ctx.childExpr())
	}

	if ctx.useFallback && ctx.mod.fallback != nil {
		if rhs, ok := ctx.mod.fallback[name]; ok {
			return h.inferExpr(rhs, ctx.childExpr())
		}
	}

	return inferredValue{}
}

func (h *Host) inferLoadedSymbol(lb loadBinding, ctx *inferContext) inferredValue {
	if ctx.mod == nil || ctx.mod.path == "" || ctx.loadDepth >= maxInferLoadDepth {
		return inferredValue{}
	}
	resolvedPath, ok := h.ResolveLoad(starlsp.LoadRequest{
		Target: lb.target,
		From:   ctx.mod.path,
	})
	if !ok {
		return inferredValue{}
	}
	targetMod := h.loadModule(resolvedPath)
	if targetMod == nil {
		return inferredValue{}
	}
	visitKey := "load:" + resolvedPath + ":" + lb.export
	if ctx.visited[visitKey] {
		return inferredValue{}
	}
	ctx.visited[visitKey] = true
	defer delete(ctx.visited, visitKey)

	return h.inferIdent(lb.export, &inferContext{
		mod:       targetMod,
		exprDepth: ctx.exprDepth + 1,
		loadDepth: ctx.loadDepth + 1,
		visited:   ctx.visited,
	})
}

func (h *Host) inferDot(dot *syntax.DotExpr, ctx *inferContext) inferredValue {
	if dot == nil || dot.Name == nil {
		return inferredValue{}
	}
	if dotted, ok := dottedIdent(dot); ok && h.cat != nil && h.cat.IsModule(dotted) {
		return inferredValue{kind: starlsp.KindProperty, valueOwner: dotted, returnsText: dotted}
	}

	recv := h.inferExpr(dot.X, ctx.childExpr())
	if recv.valueOwner == "" && ctx.resolveCB != nil {
		if owner, ok := ctx.resolveCB(exprSource(dot.X)); ok {
			recv.valueOwner = owner
		}
	}
	if recv.valueOwner == "" {
		return inferredValue{}
	}

	if schema := h.lookupStructSchema(recv.valueOwner); schema != nil {
		m, ok := schema.byName[dot.Name.Name]
		if !ok {
			return inferredValue{}
		}
		return inferredValue{
			kind:        m.symbol.Kind,
			valueOwner:  m.valueOwner,
			returnOwner: m.returnOwner,
			signature:   m.symbol.Signature,
			returnsText: m.symbol.Returns,
			doc:         m.symbol.Doc,
		}
	}

	if h.cat != nil {
		if ret, ok := h.cat.FactoryReturns(recv.valueOwner + "." + dot.Name.Name); ok {
			return inferredValue{
				kind:       starlsp.KindProperty,
				valueOwner: ret,
			}
		}
	}
	return inferredValue{}
}

func (h *Host) inferCall(call *syntax.CallExpr, ctx *inferContext) inferredValue {
	if call == nil || call.Fn == nil {
		return inferredValue{}
	}

	// Direct builtin constructor `struct(a = 1, ...)`
	if id, ok := call.Fn.(*syntax.Ident); ok && id.Name == "struct" {
		return h.inferStructLiteral(call.Args, ctx)
	}

	// Dotted catalog factory or method call, or user struct callable field.
	if dot, ok := call.Fn.(*syntax.DotExpr); ok && dot.Name != nil {
		if dotted, isDotted := dottedIdent(dot); isDotted && h.cat != nil {
			if ret, ok := h.cat.FactoryReturns(dotted); ok {
				return inferredValue{kind: starlsp.KindProperty, valueOwner: ret, returnsText: ret}
			}
		}

		recv := h.inferExpr(dot.X, ctx.childExpr())
		if recv.valueOwner == "" && ctx.resolveCB != nil {
			if owner, ok := ctx.resolveCB(exprSource(dot.X)); ok {
				recv.valueOwner = owner
			}
		}
		if schema := h.lookupStructSchema(recv.valueOwner); schema != nil {
			if m, ok := schema.byName[dot.Name.Name]; ok && m.returnOwner != "" {
				return inferredValue{
					kind:        starlsp.KindProperty,
					valueOwner:  m.returnOwner,
					returnsText: displayOwner(m.returnOwner),
				}
			}
		}
		if recv.valueOwner != "" && h.cat != nil {
			if ret, ok := h.cat.FactoryReturns(recv.valueOwner + "." + dot.Name.Name); ok {
				return inferredValue{kind: starlsp.KindProperty, valueOwner: ret, returnsText: ret}
			}
		}
		return inferredValue{}
	}

	// Identifier call: catalog factory alias or local/loaded def function.
	if id, ok := call.Fn.(*syntax.Ident); ok {
		if h.cat != nil {
			if ret, ok := h.cat.FactoryReturns(id.Name); ok {
				return inferredValue{kind: starlsp.KindProperty, valueOwner: ret, returnsText: ret}
			}
		}
	}

	fnVal := h.inferExpr(call.Fn, ctx.childExpr())
	if fnVal.returnOwner != "" {
		return inferredValue{
			kind:        starlsp.KindProperty,
			valueOwner:  fnVal.returnOwner,
			returnsText: displayOwner(fnVal.returnOwner),
		}
	}
	return inferredValue{}
}

func (h *Host) inferStructLiteral(args []syntax.Expr, ctx *inferContext) inferredValue {
	byName := make(map[string]structMember, len(args))
	child := ctx.childExpr()

	for _, arg := range args {
		kw, ok := arg.(*syntax.BinaryExpr)
		if !ok || kw.Op != syntax.EQ {
			continue
		}
		key, ok := kw.X.(*syntax.Ident)
		if !ok || key == nil {
			continue
		}
		val := h.inferExpr(kw.Y, child)
		kind := val.kind
		if kind != starlsp.KindFunction {
			kind = starlsp.KindProperty
		}

		sig := val.signature
		ret := val.returnsText
		if kind == starlsp.KindFunction {
			if val.defStmt != nil {
				sig = formatDefSignature(key.Name, val.defStmt)
			} else if sig == "" {
				sig = key.Name + "()"
			}
			if val.returnOwner != "" {
				ret = displayOwner(val.returnOwner)
			}
		} else if ret == "" && val.valueOwner != "" {
			ret = displayOwner(val.valueOwner)
		}

		byName[key.Name] = structMember{
			symbol: starlsp.Symbol{
				Name:      key.Name,
				Kind:      kind,
				Signature: sig,
				Returns:   ret,
				Doc:       val.doc,
			},
			valueOwner:  val.valueOwner,
			returnOwner: val.returnOwner,
		}
	}

	members := make([]structMember, 0, len(byName))
	for _, m := range byName {
		members = append(members, m)
	}
	owner := h.putStructSchema(members)
	return inferredValue{
		kind:        starlsp.KindProperty,
		valueOwner:  owner,
		returnsText: "struct",
	}
}

func (h *Host) inferDefSymbol(name string, defStmt *syntax.DefStmt, ctx *inferContext) inferredValue {
	sig := formatDefSignature(name, defStmt)
	doc := extractDefDocstring(defStmt)
	retOwner := h.inferDefReturnOwner(defStmt, ctx)
	return inferredValue{
		kind:        starlsp.KindFunction,
		returnOwner: retOwner,
		signature:   sig,
		returnsText: displayOwner(retOwner),
		doc:         doc,
		defStmt:     defStmt,
	}
}

func (h *Host) inferDefReturnOwner(defStmt *syntax.DefStmt, ctx *inferContext) string {
	if defStmt == nil || ctx.exprDepth >= maxInferExprDepth {
		return ""
	}
	visitKey := "def:"
	if ctx.mod != nil {
		visitKey += ctx.mod.path + ":"
	}
	if defStmt.Name != nil {
		visitKey += defStmt.Name.Name
	}
	if ctx.visited[visitKey] {
		return ""
	}
	ctx.visited[visitKey] = true
	defer delete(ctx.visited, visitKey)

	locals := make(map[string]syntax.Expr)
	collectInnerAssigns(defStmt.Body, locals)

	bodyCtx := &inferContext{
		mod:       ctx.mod,
		locals:    locals,
		exprDepth: ctx.exprDepth + 1,
		loadDepth: ctx.loadDepth,
		visited:   ctx.visited,
		resolveCB: ctx.resolveCB,
	}

	returns := collectReturnExprs(defStmt.Body)
	var owner string
	for _, retExpr := range returns {
		val := h.inferExpr(retExpr, bodyCtx.childExpr())
		if val.valueOwner == "" {
			continue
		}
		if owner == "" {
			owner = val.valueOwner
			continue
		}
		if owner == val.valueOwner {
			continue
		}
		if merged := h.mergeStructSchemas(owner, val.valueOwner); merged != "" {
			owner = merged
		}
	}
	return owner
}

func collectReturnExprs(stmts []syntax.Stmt) []syntax.Expr {
	var out []syntax.Expr
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *syntax.ReturnStmt:
			if s.Result != nil {
				out = append(out, s.Result)
			}
		case *syntax.IfStmt:
			out = append(out, collectReturnExprs(s.True)...)
			out = append(out, collectReturnExprs(s.False)...)
		case *syntax.ForStmt:
			out = append(out, collectReturnExprs(s.Body)...)
		case *syntax.WhileStmt:
			out = append(out, collectReturnExprs(s.Body)...)
		}
	}
	return out
}

func displayOwner(owner string) string {
	if strings.HasPrefix(owner, structOwnerPrefix) {
		return "struct"
	}
	return owner
}

func literalTypeName(lit *syntax.Literal) string {
	switch lit.Token {
	case syntax.STRING:
		return "string"
	case syntax.BYTES:
		return "bytes"
	case syntax.INT:
		return "int"
	case syntax.FLOAT:
		return "float"
	default:
		return ""
	}
}

func dottedIdent(dot *syntax.DotExpr) (string, bool) {
	if dot == nil || dot.Name == nil {
		return "", false
	}
	switch x := dot.X.(type) {
	case *syntax.Ident:
		return x.Name + "." + dot.Name.Name, true
	case *syntax.DotExpr:
		prefix, ok := dottedIdent(x)
		if !ok {
			return "", false
		}
		return prefix + "." + dot.Name.Name, true
	default:
		return "", false
	}
}

func extractDefDocstring(defStmt *syntax.DefStmt) string {
	if defStmt == nil || len(defStmt.Body) == 0 {
		return ""
	}
	es, ok := defStmt.Body[0].(*syntax.ExprStmt)
	if !ok {
		return ""
	}
	lit, ok := es.X.(*syntax.Literal)
	if !ok || lit.Token != syntax.STRING {
		return ""
	}
	s, ok := lit.Value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func formatDefSignature(name string, defStmt *syntax.DefStmt) string {
	if defStmt == nil {
		return name + "()"
	}
	params := make([]string, 0, len(defStmt.Params))
	for _, p := range defStmt.Params {
		params = append(params, formatParamExpr(p))
	}
	return name + "(" + strings.Join(params, ", ") + ")"
}

func formatParamExpr(p syntax.Expr) string {
	switch node := p.(type) {
	case *syntax.Ident:
		return node.Name
	case *syntax.BinaryExpr:
		if node.Op == syntax.EQ {
			return formatParamExpr(node.X) + "=" + exprSource(node.Y)
		}
	case *syntax.UnaryExpr:
		switch node.Op {
		case syntax.STAR:
			if node.X == nil {
				return "*"
			}
			return "*" + exprSource(node.X)
		case syntax.STARSTAR:
			return "**" + exprSource(node.X)
		}
	}
	return exprSource(p)
}

func exprSource(e syntax.Expr) string {
	switch node := e.(type) {
	case nil:
		return ""
	case *syntax.Ident:
		return node.Name
	case *syntax.Literal:
		if node.Raw != "" {
			return node.Raw
		}
		return "..."
	case *syntax.DotExpr:
		if node.Name == nil {
			return exprSource(node.X)
		}
		return exprSource(node.X) + "." + node.Name.Name
	case *syntax.ParenExpr:
		return "(" + exprSource(node.X) + ")"
	case *syntax.UnaryExpr:
		return node.Op.String() + exprSource(node.X)
	default:
		return "..."
	}
}
