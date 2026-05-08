package frontend

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

type preprocessState struct {
	requiredBoolHelpers map[string]struct{}
	promotedGlobals     []ast.Decl
}

// Keep constant-loop unrolling small enough that we do not explode the IR on
// wide reduction-style benchmarks; larger constant loops can stay structured.
const staticLoopUnrollLimit = 64
const staticLoopUnrollBudget = 256

func preprocessSourcesForOverlay(sources []string) (map[string][]byte, error) {
	overlay := make(map[string][]byte)
	for _, source := range sources {
		absPath, err := filepath.Abs(source)
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, absPath, content, parser.ParseComments)
		if err != nil {
			return nil, err
		}

		state := &preprocessState{requiredBoolHelpers: make(map[string]struct{})}
		changed, err := rewriteFrontendFile(file, state)
		if err != nil {
			return nil, err
		}
		if changed {
			addBoolHelpers(file, state)
			var buf bytes.Buffer
			if err := format.Node(&buf, fset, file); err != nil {
				return nil, err
			}
			overlay[absPath] = buf.Bytes()
		}
	}
	if len(overlay) == 0 {
		return nil, nil
	}
	return overlay, nil
}

func rewriteFrontendFile(file *ast.File, state *preprocessState) (bool, error) {
	changed := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		fnChanged := rewriteClockShadowConditions(fn.Body)
		promoted := promoteClockedLocalState(file, fn, state)
		typeHints := collectBoolTypeHints(fn)
		unused := collectUnusedLocalNames(fn)
		bodyChanged := rewriteStmtListForFrontend(&fn.Body.List, unused, typeHints, state)
		changed = changed || fnChanged || promoted || bodyChanged
	}
	if len(state.promotedGlobals) > 0 {
		file.Decls = append(append([]ast.Decl{}, state.promotedGlobals...), file.Decls...)
		changed = true
	}
	return changed, nil
}

type promotedLocal struct {
	name       string
	obj        *ast.Object
	globalName string
}

func promoteClockedLocalState(file *ast.File, fn *ast.FuncDecl, state *preprocessState) bool {
	if file == nil || fn == nil || fn.Body == nil || state == nil {
		return false
	}
	assignedInClocked := make(map[string]struct{})
	collectClockedAssignments(fn.Body.List, false, assignedInClocked)
	if len(assignedInClocked) == 0 && functionHasClockParam(fn) {
		collectImplicitClockStateCandidates(fn, assignedInClocked)
	}
	if len(assignedInClocked) == 0 {
		return false
	}
	localConsts := collectFunctionLocalConstExprs(fn)

	promoted := make(map[string]promotedLocal)
	changed := false
	for i, stmt := range fn.Body.List {
		if assign, ok := stmt.(*ast.AssignStmt); ok && assign != nil && assign.Tok == token.DEFINE && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
			ident, ok := assign.Lhs[0].(*ast.Ident)
			if ok && ident != nil && ident.Name != "_" {
				if _, ok := assignedInClocked[ident.Name]; ok {
					globalName := promotedGlobalName(fn.Name.Name, ident.Name)
					values := []ast.Expr{}
					if initExpr := rewritePromotedGlobalInitExpr(assign.Rhs[0], localConsts); initExpr != nil {
						values = []ast.Expr{initExpr}
					}
					state.promotedGlobals = append(state.promotedGlobals, &ast.GenDecl{
						Tok: token.VAR,
						Specs: []ast.Spec{&ast.ValueSpec{
							Names:  []*ast.Ident{ast.NewIdent(globalName)},
							Values: values,
						}},
					})
					promoted[ident.Name] = promotedLocal{
						name:       ident.Name,
						obj:        ident.Obj,
						globalName: globalName,
					}
					fn.Body.List[i] = &ast.EmptyStmt{}
					changed = true
					continue
				}
			}
		}
		declStmt, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		gen, ok := declStmt.Decl.(*ast.GenDecl)
		if !ok || gen == nil || gen.Tok != token.VAR {
			continue
		}
		newSpecs := make([]ast.Spec, 0, len(gen.Specs))
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || valueSpec == nil {
				newSpecs = append(newSpecs, spec)
				continue
			}
			remaining := make([]*ast.Ident, 0, len(valueSpec.Names))
			for idx, ident := range valueSpec.Names {
				if ident == nil || ident.Name == "_" {
					continue
				}
				if _, ok := assignedInClocked[ident.Name]; !ok {
					remaining = append(remaining, ident)
					continue
				}
				globalName := promotedGlobalName(fn.Name.Name, ident.Name)
				state.promotedGlobals = append(state.promotedGlobals, buildPromotedGlobalDecl(globalName, valueSpec, idx, localConsts))
				promoted[ident.Name] = promotedLocal{
					name:       ident.Name,
					obj:        ident.Obj,
					globalName: globalName,
				}
				changed = true
			}
			if len(remaining) == 0 {
				continue
			}
			cloned := *valueSpec
			cloned.Names = remaining
			newSpecs = append(newSpecs, &cloned)
		}
		if len(newSpecs) == 0 {
			fn.Body.List[i] = &ast.EmptyStmt{}
			continue
		}
		cloned := *gen
		cloned.Specs = newSpecs
		fn.Body.List[i] = &ast.DeclStmt{Decl: &cloned}
	}
	if !changed {
		return false
	}

	astutil.Apply(fn.Body, func(c *astutil.Cursor) bool {
		ident, ok := c.Node().(*ast.Ident)
		if !ok || ident == nil {
			return true
		}
		entry, ok := promoted[ident.Name]
		if !ok {
			return true
		}
		if ident.Obj != nil && entry.obj != nil && ident.Obj != entry.obj {
			return true
		}
		c.Replace(ast.NewIdent(entry.globalName))
		return false
	}, nil)
	return true
}

func functionHasClockParam(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		if field == nil {
			continue
		}
		for _, name := range field.Names {
			if name != nil && isClockName(name.Name) {
				return true
			}
		}
	}
	return false
}

func buildPromotedGlobalDecl(globalName string, spec *ast.ValueSpec, index int, localConsts map[string]ast.Expr) ast.Decl {
	valueSpec := &ast.ValueSpec{
		Names: []*ast.Ident{ast.NewIdent(globalName)},
	}
	if spec != nil {
		valueSpec.Type = spec.Type
		if index >= 0 && index < len(spec.Values) {
			if initExpr := rewritePromotedGlobalInitExpr(spec.Values[index], localConsts); initExpr != nil {
				valueSpec.Values = []ast.Expr{initExpr}
			}
		}
	}
	return &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{valueSpec},
	}
}

func rewritePromotedGlobalInitExpr(expr ast.Expr, localConsts map[string]ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}
	rewritten := cloneExpr(expr)
	if rewritten == nil {
		return nil
	}
	if len(localConsts) == 0 {
		return rewritten
	}
	replaced := astutil.Apply(rewritten, func(c *astutil.Cursor) bool {
		ident, ok := c.Node().(*ast.Ident)
		if !ok || ident == nil {
			return true
		}
		replacement, ok := localConsts[ident.Name]
		if !ok || replacement == nil {
			return true
		}
		c.Replace(cloneExpr(replacement))
		return false
	}, nil)
	next, ok := replaced.(ast.Expr)
	if !ok {
		return nil
	}
	if exprContainsLocalConstReference(next, localConsts) {
		return nil
	}
	return next
}

func exprContainsLocalConstReference(expr ast.Expr, localConsts map[string]ast.Expr) bool {
	if expr == nil || len(localConsts) == 0 {
		return false
	}
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if !ok || ident == nil {
			return true
		}
		if ident.Name == "iota" {
			found = true
			return false
		}
		if _, ok := localConsts[ident.Name]; ok {
			found = true
			return false
		}
		return true
	})
	return found
}

func collectFunctionLocalConstExprs(fn *ast.FuncDecl) map[string]ast.Expr {
	consts := make(map[string]ast.Expr)
	if fn == nil || fn.Body == nil {
		return consts
	}
	for _, stmt := range fn.Body.List {
		declStmt, ok := stmt.(*ast.DeclStmt)
		if !ok || declStmt == nil {
			continue
		}
		gen, ok := declStmt.Decl.(*ast.GenDecl)
		if !ok || gen == nil || gen.Tok != token.CONST {
			continue
		}
		var repeated []ast.Expr
		iotaValue := int64(0)
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || valueSpec == nil || len(valueSpec.Names) == 0 {
				iotaValue++
				continue
			}
			exprs := valueSpec.Values
			if len(exprs) == 0 {
				exprs = repeated
			} else {
				repeated = exprs
			}
			for idx, name := range valueSpec.Names {
				if name == nil || name.Name == "_" {
					continue
				}
				expr := repeatedConstExpr(exprs, idx)
				if expr == nil {
					continue
				}
				resolved, ok := resolveLocalConstExpr(expr, consts, iotaValue)
				if !ok || resolved == nil {
					continue
				}
				consts[name.Name] = resolved
			}
			iotaValue++
		}
	}
	return consts
}

func repeatedConstExpr(exprs []ast.Expr, idx int) ast.Expr {
	if len(exprs) == 0 {
		return nil
	}
	if idx < len(exprs) {
		return exprs[idx]
	}
	return exprs[len(exprs)-1]
}

func resolveLocalConstExpr(expr ast.Expr, known map[string]ast.Expr, iotaValue int64) (ast.Expr, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return cloneExpr(e), true
	case *ast.Ident:
		switch e.Name {
		case "iota":
			return &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(iotaValue, 10)}, true
		case "true", "false":
			return ast.NewIdent(e.Name), true
		default:
			resolved, ok := known[e.Name]
			if !ok || resolved == nil {
				return nil, false
			}
			return cloneExpr(resolved), true
		}
	case *ast.ParenExpr:
		x, ok := resolveLocalConstExpr(e.X, known, iotaValue)
		if !ok {
			return nil, false
		}
		return &ast.ParenExpr{X: x}, true
	case *ast.UnaryExpr:
		x, ok := resolveLocalConstExpr(e.X, known, iotaValue)
		if !ok {
			return nil, false
		}
		return &ast.UnaryExpr{Op: e.Op, X: x}, true
	case *ast.BinaryExpr:
		x, ok := resolveLocalConstExpr(e.X, known, iotaValue)
		if !ok {
			return nil, false
		}
		y, ok := resolveLocalConstExpr(e.Y, known, iotaValue)
		if !ok {
			return nil, false
		}
		return &ast.BinaryExpr{X: x, Op: e.Op, Y: y}, true
	case *ast.CallExpr:
		args := make([]ast.Expr, 0, len(e.Args))
		for _, arg := range e.Args {
			resolved, ok := resolveLocalConstExpr(arg, known, iotaValue)
			if !ok {
				return nil, false
			}
			args = append(args, resolved)
		}
		return &ast.CallExpr{Fun: cloneExpr(e.Fun), Args: args}, true
	default:
		return nil, false
	}
}

func promotedGlobalName(funcName, localName string) string {
	funcName = sanitizePromotedName(funcName)
	localName = sanitizePromotedName(localName)
	return "__mygo_state_" + funcName + "_" + localName
}

func sanitizePromotedName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "state"
	}
	var b strings.Builder
	for _, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "state"
	}
	return b.String()
}

func collectClockedAssignments(list []ast.Stmt, inClocked bool, assigned map[string]struct{}) {
	for _, stmt := range list {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			collectClockedAssignments(s.List, inClocked, assigned)
		case *ast.IfStmt:
			clocked := inClocked || exprLooksClockGuard(s.Cond)
			if s.Init != nil {
				collectClockedAssignments([]ast.Stmt{s.Init}, inClocked, assigned)
			}
			collectClockedAssignments(s.Body.List, clocked, assigned)
			if s.Else != nil {
				collectClockedAssignments([]ast.Stmt{s.Else}, clocked, assigned)
			}
		case *ast.ForStmt:
			clocked := inClocked || exprLooksClockGuard(s.Cond)
			collectClockedAssignments(s.Body.List, clocked, assigned)
		case *ast.RangeStmt:
			collectClockedAssignments(s.Body.List, inClocked, assigned)
		case *ast.SwitchStmt:
			clocked := inClocked || exprLooksClockGuard(s.Tag)
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok {
					continue
				}
				collectClockedAssignments(cc.Body, clocked, assigned)
			}
		case *ast.AssignStmt:
			if !inClocked {
				continue
			}
			for _, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident != nil && ident.Name != "_" {
					assigned[ident.Name] = struct{}{}
				}
			}
		case *ast.IncDecStmt:
			if !inClocked {
				continue
			}
			if ident, ok := s.X.(*ast.Ident); ok && ident != nil && ident.Name != "_" {
				assigned[ident.Name] = struct{}{}
			}
		}
	}
}

func collectAssignments(list []ast.Stmt, assigned map[string]struct{}) {
	for _, stmt := range list {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			collectAssignments(s.List, assigned)
		case *ast.IfStmt:
			if s.Init != nil {
				collectAssignments([]ast.Stmt{s.Init}, assigned)
			}
			collectAssignments(s.Body.List, assigned)
			if s.Else != nil {
				collectAssignments([]ast.Stmt{s.Else}, assigned)
			}
		case *ast.ForStmt:
			if s.Init != nil {
				collectAssignments([]ast.Stmt{s.Init}, assigned)
			}
			if s.Post != nil {
				collectAssignments([]ast.Stmt{s.Post}, assigned)
			}
			collectAssignments(s.Body.List, assigned)
		case *ast.RangeStmt:
			collectAssignments(s.Body.List, assigned)
		case *ast.SwitchStmt:
			if s.Init != nil {
				collectAssignments([]ast.Stmt{s.Init}, assigned)
			}
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok {
					continue
				}
				collectAssignments(cc.Body, assigned)
			}
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident != nil && ident.Name != "_" {
					assigned[ident.Name] = struct{}{}
				}
			}
		case *ast.IncDecStmt:
			if ident, ok := s.X.(*ast.Ident); ok && ident != nil && ident.Name != "_" {
				assigned[ident.Name] = struct{}{}
			}
		}
	}
}

type implicitStateCandidate struct {
	hasExplicitInit bool
	initExpr        ast.Expr
	selfReferenced  bool
}

func collectImplicitClockStateCandidates(fn *ast.FuncDecl, assigned map[string]struct{}) {
	if fn == nil || fn.Body == nil || assigned == nil {
		return
	}
	localConsts := collectFunctionLocalConstExprs(fn)
	candidates := make(map[string]*implicitStateCandidate)
	collectImplicitStateDecls(fn.Body.List, candidates)
	if len(candidates) == 0 {
		return
	}
	collectAssignments(fn.Body.List, assigned)
	collectImplicitStateSelfReferences(fn.Body.List, candidates)
	for name, candidate := range candidates {
		if candidate == nil {
			delete(assigned, name)
			continue
		}
		if candidate.hasExplicitInit {
			if candidate.initExpr == nil || !canPromoteImplicitStateInit(candidate.initExpr, localConsts) {
				delete(assigned, name)
			}
			continue
		}
		if !candidate.selfReferenced {
			delete(assigned, name)
		}
	}
}

func collectImplicitStateDecls(list []ast.Stmt, candidates map[string]*implicitStateCandidate) {
	for _, stmt := range list {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			collectImplicitStateDecls(s.List, candidates)
		case *ast.IfStmt:
			if s.Init != nil {
				collectImplicitStateDecls([]ast.Stmt{s.Init}, candidates)
			}
			collectImplicitStateDecls(s.Body.List, candidates)
			if s.Else != nil {
				collectImplicitStateDecls([]ast.Stmt{s.Else}, candidates)
			}
		case *ast.ForStmt:
			if s.Init != nil {
				collectImplicitStateDecls([]ast.Stmt{s.Init}, candidates)
			}
			if s.Post != nil {
				collectImplicitStateDecls([]ast.Stmt{s.Post}, candidates)
			}
			collectImplicitStateDecls(s.Body.List, candidates)
		case *ast.RangeStmt:
			collectImplicitStateDecls(s.Body.List, candidates)
		case *ast.SwitchStmt:
			if s.Init != nil {
				collectImplicitStateDecls([]ast.Stmt{s.Init}, candidates)
			}
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok {
					continue
				}
				collectImplicitStateDecls(cc.Body, candidates)
			}
		case *ast.AssignStmt:
			if s.Tok != token.DEFINE {
				continue
			}
			for i, lhs := range s.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident == nil || ident.Name == "_" {
					continue
				}
				candidate := &implicitStateCandidate{}
				if i < len(s.Rhs) && s.Rhs[i] != nil {
					candidate.hasExplicitInit = true
					candidate.initExpr = s.Rhs[i]
				}
				candidates[ident.Name] = candidate
			}
		case *ast.DeclStmt:
			gen, ok := s.Decl.(*ast.GenDecl)
			if !ok || gen == nil || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || valueSpec == nil {
					continue
				}
				for i, name := range valueSpec.Names {
					if name == nil || name.Name == "_" {
						continue
					}
					candidate := &implicitStateCandidate{}
					if i < len(valueSpec.Values) && valueSpec.Values[i] != nil {
						candidate.hasExplicitInit = true
						candidate.initExpr = valueSpec.Values[i]
					}
					candidates[name.Name] = candidate
				}
			}
		}
	}
}

func collectImplicitStateSelfReferences(list []ast.Stmt, candidates map[string]*implicitStateCandidate) {
	for _, stmt := range list {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			collectImplicitStateSelfReferences(s.List, candidates)
		case *ast.IfStmt:
			markCandidateExprReferences(s.Cond, candidates)
			if s.Init != nil {
				collectImplicitStateSelfReferences([]ast.Stmt{s.Init}, candidates)
			}
			collectImplicitStateSelfReferences(s.Body.List, candidates)
			if s.Else != nil {
				collectImplicitStateSelfReferences([]ast.Stmt{s.Else}, candidates)
			}
		case *ast.ForStmt:
			if s.Init != nil {
				collectImplicitStateSelfReferences([]ast.Stmt{s.Init}, candidates)
			}
			if s.Cond != nil {
				markCandidateExprReferences(s.Cond, candidates)
			}
			if s.Post != nil {
				collectImplicitStateSelfReferences([]ast.Stmt{s.Post}, candidates)
			}
			collectImplicitStateSelfReferences(s.Body.List, candidates)
		case *ast.RangeStmt:
			collectImplicitStateSelfReferences(s.Body.List, candidates)
		case *ast.SwitchStmt:
			if s.Init != nil {
				collectImplicitStateSelfReferences([]ast.Stmt{s.Init}, candidates)
			}
			if s.Tag != nil {
				markCandidateExprReferences(s.Tag, candidates)
			}
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range cc.List {
					markCandidateExprReferences(expr, candidates)
				}
				collectImplicitStateSelfReferences(cc.Body, candidates)
			}
		case *ast.AssignStmt:
			lhsNames := make(map[string]struct{})
			for _, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident != nil {
					lhsNames[ident.Name] = struct{}{}
				}
			}
			for name := range lhsNames {
				candidate, ok := candidates[name]
				if !ok || candidate == nil {
					continue
				}
				for _, rhs := range s.Rhs {
					if exprReferencesName(rhs, name) {
						candidate.selfReferenced = true
					}
				}
			}
		case *ast.IncDecStmt:
			if ident, ok := s.X.(*ast.Ident); ok && ident != nil {
				if candidate, ok := candidates[ident.Name]; ok && candidate != nil {
					candidate.selfReferenced = true
				}
			}
		}
	}
}

func markCandidateExprReferences(expr ast.Expr, candidates map[string]*implicitStateCandidate) {
	if expr == nil || len(candidates) == 0 {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident == nil {
			return true
		}
		if candidate, ok := candidates[ident.Name]; ok && candidate != nil {
			candidate.selfReferenced = true
		}
		return true
	})
}

func exprReferencesName(expr ast.Expr, name string) bool {
	if expr == nil || strings.TrimSpace(name) == "" {
		return false
	}
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident == nil {
			return true
		}
		if ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func canPromoteImplicitStateInit(expr ast.Expr, localConsts map[string]ast.Expr) bool {
	if expr == nil {
		return false
	}
	rewritten := cloneExpr(expr)
	if rewritten == nil {
		return false
	}
	if len(localConsts) == 0 {
		return exprIsConstLike(rewritten)
	}
	replaced := astutil.Apply(rewritten, func(c *astutil.Cursor) bool {
		ident, ok := c.Node().(*ast.Ident)
		if !ok || ident == nil {
			return true
		}
		replacement, ok := localConsts[ident.Name]
		if !ok || replacement == nil {
			return true
		}
		c.Replace(cloneExpr(replacement))
		return false
	}, nil)
	next, ok := replaced.(ast.Expr)
	if !ok || next == nil {
		return false
	}
	return exprIsConstLike(next)
}

func exprIsConstLike(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return e.Name == "true" || e.Name == "false"
	case *ast.ParenExpr:
		return exprIsConstLike(e.X)
	case *ast.UnaryExpr:
		return exprIsConstLike(e.X)
	case *ast.BinaryExpr:
		return exprIsConstLike(e.X) && exprIsConstLike(e.Y)
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); !ok || ident == nil {
			return false
		}
		for _, arg := range e.Args {
			if !exprIsConstLike(arg) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func exprLooksClockGuard(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return isClockName(e.Name)
	case *ast.ParenExpr:
		return exprLooksClockGuard(e.X)
	case *ast.UnaryExpr:
		return e.Op == token.NOT && exprLooksClockGuard(e.X)
	default:
		return false
	}
}

func isClockName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "clk", "clock":
		return true
	default:
		return false
	}
}

func rewriteClockShadowConditions(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	changed := false
	for _, stmt := range block.List {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			if rewriteClockShadowEdgeCond(s) {
				changed = true
			}
			if rewriteClockShadowConditions(s.Body) {
				changed = true
			}
			if s.Else != nil {
				if rewriteElseClockShadow(s.Else) {
					changed = true
				}
			}
		case *ast.BlockStmt:
			if rewriteClockShadowConditions(s) {
				changed = true
			}
		case *ast.ForStmt:
			if rewriteClockShadowConditions(s.Body) {
				changed = true
			}
		case *ast.SwitchStmt:
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok {
					continue
				}
				if rewriteCaseClockShadow(cc.Body) {
					changed = true
				}
			}
		}
	}
	return changed
}

func rewriteElseClockShadow(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		return rewriteClockShadowConditions(s)
	case *ast.IfStmt:
		changed := rewriteClockShadowEdgeCond(s)
		if rewriteClockShadowConditions(s.Body) {
			changed = true
		}
		if s.Else != nil && rewriteElseClockShadow(s.Else) {
			changed = true
		}
		return changed
	default:
		return false
	}
}

func rewriteCaseClockShadow(list []ast.Stmt) bool {
	changed := false
	for _, stmt := range list {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			if rewriteClockShadowConditions(s) {
				changed = true
			}
		case *ast.IfStmt:
			if rewriteClockShadowEdgeCond(s) {
				changed = true
			}
			if rewriteClockShadowConditions(s.Body) {
				changed = true
			}
			if s.Else != nil && rewriteElseClockShadow(s.Else) {
				changed = true
			}
		}
	}
	return changed
}

func rewriteClockShadowEdgeCond(stmt *ast.IfStmt) bool {
	if stmt == nil || stmt.Cond == nil {
		return false
	}
	and, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok || and.Op != token.LAND {
		return false
	}
	leftNot, ok := and.X.(*ast.UnaryExpr)
	if !ok || leftNot.Op != token.NOT {
		return false
	}
	leftIdent, ok := leftNot.X.(*ast.Ident)
	if !ok || leftIdent == nil {
		return false
	}
	rightIdent, ok := and.Y.(*ast.Ident)
	if !ok || rightIdent == nil {
		return false
	}
	if leftIdent.Name != "prev_"+rightIdent.Name {
		return false
	}
	stmt.Cond = ast.NewIdent(rightIdent.Name)
	return true
}

func rewriteStmtListForFrontend(list *[]ast.Stmt, unused map[string]struct{}, hints *boolTypeHints, state *preprocessState) bool {
	if list == nil {
		return false
	}
	changed := false
	var out []ast.Stmt
	stmts := *list
	for i := 0; i < len(stmts); i++ {
		stmt := stmts[i]
		if rewritten, ok := rewriteBooleanMuxAssign(stmt); ok {
			stmt = rewritten
			changed = true
		}
		if forStmt, ok := stmt.(*ast.ForStmt); ok {
			if expanded, ok := tryUnrollConstFor(forStmt); ok {
				for _, expandedStmt := range expanded {
					if rewriteStmtForFrontend(expandedStmt, unused, hints, state) {
						changed = true
					}
					out = append(out, expandedStmt)
				}
				changed = true
				continue
			}
		}
		if rewriteStmtForFrontend(stmt, unused, hints, state) {
			changed = true
		}
		out = append(out, stmt)
		if defs := declaredNamesInStmt(stmt); len(defs) > 0 {
			for _, name := range defs {
				if _, ok := unused[name]; ok {
					out = append(out, blankUseStmt(name))
					changed = true
				}
			}
		}
	}
	if changed {
		*list = out
	}
	return changed
}

func rewriteBooleanMuxAssign(stmt ast.Stmt) (ast.Stmt, bool) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || assign == nil || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return stmt, false
	}
	orExpr, ok := assign.Rhs[0].(*ast.BinaryExpr)
	if !ok || orExpr == nil || orExpr.Op != token.LOR {
		return stmt, false
	}
	left, ok := orExpr.X.(*ast.BinaryExpr)
	if !ok || left == nil || left.Op != token.LAND {
		return stmt, false
	}
	right, ok := orExpr.Y.(*ast.BinaryExpr)
	if !ok || right == nil || right.Op != token.LAND {
		return stmt, false
	}
	neg, ok := right.X.(*ast.UnaryExpr)
	if !ok || neg == nil || neg.Op != token.NOT {
		return stmt, false
	}
	if !sameASTExpr(left.X, neg.X) {
		return stmt, false
	}
	thenAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{cloneExpr(assign.Lhs[0])},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{cloneExpr(left.Y)},
	}
	elseAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{cloneExpr(assign.Lhs[0])},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{cloneExpr(right.Y)},
	}
	return &ast.IfStmt{
		Cond: cloneExpr(left.X),
		Body: &ast.BlockStmt{List: []ast.Stmt{thenAssign}},
		Else: &ast.BlockStmt{List: []ast.Stmt{elseAssign}},
	}, true
}

func sameASTExpr(a, b ast.Expr) bool {
	if a == nil || b == nil {
		return a == b
	}
	var abuf, bbuf bytes.Buffer
	if err := format.Node(&abuf, token.NewFileSet(), a); err != nil {
		return false
	}
	if err := format.Node(&bbuf, token.NewFileSet(), b); err != nil {
		return false
	}
	return abuf.String() == bbuf.String()
}

func cloneExpr(expr ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
		return expr
	}
	parsed, err := parser.ParseExpr(buf.String())
	if err != nil {
		return expr
	}
	return parsed
}

type constForLoopSpec struct {
	name string
	cur  int64
	end  int64
	step int64
	op   token.Token
}

func tryUnrollConstFor(stmt *ast.ForStmt) ([]ast.Stmt, bool) {
	spec, ok := parseConstForLoop(stmt)
	if !ok || stmt == nil || stmt.Body == nil {
		return nil, false
	}
	if loopBodyHasDisallowedControl(stmt.Body) || loopBodyWritesName(stmt.Body, spec.name) {
		return nil, false
	}
	var tripCount int
	for cur := spec.cur; constForCondHolds(cur, spec.end, spec.op); cur += spec.step {
		tripCount++
		if tripCount > staticLoopUnrollLimit {
			return nil, false
		}
	}
	bodyCost := estimateStmtListCost(stmt.Body.List)
	if bodyCost <= 0 {
		bodyCost = 1
	}
	if tripCount*bodyCost > staticLoopUnrollBudget {
		return nil, false
	}
	if tripCount == 0 {
		return []ast.Stmt{}, true
	}
	expanded := make([]ast.Stmt, 0, len(stmt.Body.List)*tripCount)
	for cur := spec.cur; constForCondHolds(cur, spec.end, spec.op); cur += spec.step {
		cloned, ok := cloneStmtList(stmt.Body.List)
		if !ok {
			return nil, false
		}
		substituteLoopVar(cloned, spec.name, cur)
		expanded = append(expanded, &ast.BlockStmt{List: cloned})
	}
	return expanded, true
}

func parseConstForLoop(stmt *ast.ForStmt) (constForLoopSpec, bool) {
	if stmt == nil || stmt.Init == nil || stmt.Cond == nil || stmt.Post == nil {
		return constForLoopSpec{}, false
	}
	name, start, ok := parseConstForInit(stmt.Init)
	if !ok {
		return constForLoopSpec{}, false
	}
	cond, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok {
		return constForLoopSpec{}, false
	}
	left, ok := cond.X.(*ast.Ident)
	if !ok || left.Name != name {
		return constForLoopSpec{}, false
	}
	end, ok := evalConstIntExpr(cond.Y)
	if !ok {
		return constForLoopSpec{}, false
	}
	switch cond.Op {
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return constForLoopSpec{}, false
	}
	step, ok := parseConstForStep(stmt.Post, name)
	if !ok || step == 0 {
		return constForLoopSpec{}, false
	}
	if step > 0 && !(cond.Op == token.LSS || cond.Op == token.LEQ) {
		return constForLoopSpec{}, false
	}
	if step < 0 && !(cond.Op == token.GTR || cond.Op == token.GEQ) {
		return constForLoopSpec{}, false
	}
	return constForLoopSpec{name: name, cur: start, end: end, step: step, op: cond.Op}, true
}

func parseConstForInit(stmt ast.Stmt) (string, int64, bool) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return "", 0, false
	}
	if assign.Tok != token.DEFINE {
		return "", 0, false
	}
	ident, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || ident.Name == "_" {
		return "", 0, false
	}
	value, ok := evalConstIntExpr(assign.Rhs[0])
	if !ok {
		return "", 0, false
	}
	return ident.Name, value, true
}

func parseConstForStep(stmt ast.Stmt, name string) (int64, bool) {
	switch s := stmt.(type) {
	case *ast.IncDecStmt:
		ident, ok := s.X.(*ast.Ident)
		if !ok || ident.Name != name {
			return 0, false
		}
		switch s.Tok {
		case token.INC:
			return 1, true
		case token.DEC:
			return -1, true
		default:
			return 0, false
		}
	case *ast.AssignStmt:
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return 0, false
		}
		ident, ok := s.Lhs[0].(*ast.Ident)
		if !ok || ident.Name != name {
			return 0, false
		}
		switch s.Tok {
		case token.ADD_ASSIGN:
			v, ok := evalConstIntExpr(s.Rhs[0])
			return v, ok
		case token.SUB_ASSIGN:
			v, ok := evalConstIntExpr(s.Rhs[0])
			return -v, ok
		case token.ASSIGN:
			bin, ok := s.Rhs[0].(*ast.BinaryExpr)
			if !ok {
				return 0, false
			}
			lhs, ok := bin.X.(*ast.Ident)
			if !ok || lhs.Name != name {
				return 0, false
			}
			v, ok := evalConstIntExpr(bin.Y)
			if !ok {
				return 0, false
			}
			switch bin.Op {
			case token.ADD:
				return v, true
			case token.SUB:
				return -v, true
			}
		}
	}
	return 0, false
}

func evalConstIntExpr(expr ast.Expr) (int64, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		v, err := strconv.ParseInt(e.Value, 0, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	case *ast.UnaryExpr:
		v, ok := evalConstIntExpr(e.X)
		if !ok {
			return 0, false
		}
		switch e.Op {
		case token.ADD:
			return v, true
		case token.SUB:
			return -v, true
		}
	}
	return 0, false
}

func constForCondHolds(cur, end int64, op token.Token) bool {
	switch op {
	case token.LSS:
		return cur < end
	case token.LEQ:
		return cur <= end
	case token.GTR:
		return cur > end
	case token.GEQ:
		return cur >= end
	default:
		return false
	}
}

func estimateStmtListCost(list []ast.Stmt) int {
	total := 0
	for _, stmt := range list {
		total += estimateStmtCost(stmt)
	}
	return total
}

func estimateStmtCost(stmt ast.Stmt) int {
	switch s := stmt.(type) {
	case nil:
		return 0
	case *ast.BlockStmt:
		return estimateStmtListCost(s.List)
	case *ast.ForStmt:
		spec, ok := parseConstForLoop(s)
		if !ok || s.Body == nil {
			return 1
		}
		tripCount := 0
		for cur := spec.cur; constForCondHolds(cur, spec.end, spec.op); cur += spec.step {
			tripCount++
			if tripCount > staticLoopUnrollLimit {
				return staticLoopUnrollBudget + 1
			}
		}
		bodyCost := estimateStmtListCost(s.Body.List)
		if bodyCost <= 0 {
			bodyCost = 1
		}
		return tripCount * bodyCost
	case *ast.IfStmt:
		total := 1 + estimateStmtListCost(s.Body.List)
		if s.Else != nil {
			total += estimateStmtCost(s.Else)
		}
		return total
	case *ast.SwitchStmt:
		total := 1
		for _, clause := range s.Body.List {
			cc, ok := clause.(*ast.CaseClause)
			if !ok {
				continue
			}
			total += estimateStmtListCost(cc.Body)
		}
		return total
	default:
		return 1
	}
}

func loopBodyHasDisallowedControl(body *ast.BlockStmt) bool {
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.DeclStmt:
			return true
		case *ast.AssignStmt:
			if s.Tok == token.DEFINE {
				return true
			}
		}
	}
	disallowed := false
	ast.Inspect(body, func(n ast.Node) bool {
		if disallowed || n == nil {
			return false
		}
		switch n.(type) {
		case *ast.BranchStmt, *ast.RangeStmt, *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit:
			disallowed = true
			return false
		}
		return true
	})
	return disallowed
}

func loopBodyWritesName(body *ast.BlockStmt, name string) bool {
	writes := false
	ast.Inspect(body, func(n ast.Node) bool {
		if writes || n == nil {
			return false
		}
		switch s := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					writes = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if id, ok := s.X.(*ast.Ident); ok && id.Name == name {
				writes = true
				return false
			}
		case *ast.RangeStmt:
			if id, ok := s.Key.(*ast.Ident); ok && id.Name == name {
				writes = true
				return false
			}
			if id, ok := s.Value.(*ast.Ident); ok && id.Name == name {
				writes = true
				return false
			}
		}
		return true
	})
	return writes
}

func cloneStmtList(list []ast.Stmt) ([]ast.Stmt, bool) {
	if len(list) == 0 {
		return nil, true
	}
	var buf bytes.Buffer
	buf.WriteString("package main\nfunc _(){\n")
	for _, stmt := range list {
		if err := format.Node(&buf, token.NewFileSet(), stmt); err != nil {
			return nil, false
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	file, err := parser.ParseFile(token.NewFileSet(), "", buf.Bytes(), 0)
	if err != nil || len(file.Decls) == 0 {
		return nil, false
	}
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Body == nil {
		return nil, false
	}
	return fn.Body.List, true
}

func substituteLoopVar(stmts []ast.Stmt, name string, value int64) {
	if len(stmts) == 0 || name == "" {
		return
	}
	literal := strconv.FormatInt(value, 10)
	for i, stmt := range stmts {
		replaced := astutil.Apply(stmt, func(c *astutil.Cursor) bool {
			id, ok := c.Node().(*ast.Ident)
			if !ok || id.Name != name {
				return true
			}
			if c.Name() == "Sel" {
				return false
			}
			c.Replace(&ast.BasicLit{Kind: token.INT, Value: literal})
			return false
		}, nil)
		if next, ok := replaced.(ast.Stmt); ok {
			stmts[i] = next
		}
	}
}

func rewriteStmtForFrontend(stmt ast.Stmt, unused map[string]struct{}, hints *boolTypeHints, state *preprocessState) bool {
	changed := false
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for i := range s.Rhs {
			expr, exprChanged := rewriteExprForFrontend(s.Rhs[i], hints, state)
			s.Rhs[i] = expr
			changed = changed || exprChanged
		}
		for i := range s.Lhs {
			expr, exprChanged := rewriteExprForFrontend(s.Lhs[i], hints, state)
			s.Lhs[i] = expr
			changed = changed || exprChanged
		}
	case *ast.DeclStmt:
		gen, ok := s.Decl.(*ast.GenDecl)
		if !ok {
			return false
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i := range valueSpec.Values {
				expr, exprChanged := rewriteExprForFrontend(valueSpec.Values[i], hints, state)
				valueSpec.Values[i] = expr
				changed = changed || exprChanged
			}
		}
	case *ast.IfStmt:
		if s.Init != nil && rewriteStmtForFrontend(s.Init, unused, hints, state) {
			changed = true
		}
		if expr, exprChanged := rewriteExprForFrontend(s.Cond, hints, state); exprChanged {
			s.Cond = expr
			changed = true
		}
		if rewriteStmtListForFrontend(&s.Body.List, unused, hints, state) {
			changed = true
		}
		if s.Else != nil {
			if rewriteNestedStmtForFrontend(s.Else, unused, hints, state) {
				changed = true
			}
		}
	case *ast.BlockStmt:
		if rewriteStmtListForFrontend(&s.List, unused, hints, state) {
			changed = true
		}
	case *ast.ForStmt:
		if s.Init != nil && rewriteStmtForFrontend(s.Init, unused, hints, state) {
			changed = true
		}
		if s.Cond != nil {
			if expr, exprChanged := rewriteExprForFrontend(s.Cond, hints, state); exprChanged {
				s.Cond = expr
				changed = true
			}
		}
		if s.Post != nil && rewriteStmtForFrontend(s.Post, unused, hints, state) {
			changed = true
		}
		if rewriteStmtListForFrontend(&s.Body.List, unused, hints, state) {
			changed = true
		}
	case *ast.SwitchStmt:
		if s.Init != nil && rewriteStmtForFrontend(s.Init, unused, hints, state) {
			changed = true
		}
		if s.Tag != nil {
			if expr, exprChanged := rewriteExprForFrontend(s.Tag, hints, state); exprChanged {
				s.Tag = expr
				changed = true
			}
		}
		for _, clause := range s.Body.List {
			cc, ok := clause.(*ast.CaseClause)
			if !ok {
				continue
			}
			for i := range cc.List {
				expr, exprChanged := rewriteExprForFrontend(cc.List[i], hints, state)
				cc.List[i] = expr
				changed = changed || exprChanged
			}
			if rewriteStmtListForFrontend(&cc.Body, unused, hints, state) {
				changed = true
			}
		}
	case *ast.ReturnStmt:
		for i := range s.Results {
			expr, exprChanged := rewriteExprForFrontend(s.Results[i], hints, state)
			s.Results[i] = expr
			changed = changed || exprChanged
		}
	case *ast.ExprStmt:
		if expr, exprChanged := rewriteExprForFrontend(s.X, hints, state); exprChanged {
			s.X = expr
			changed = true
		}
	}
	return changed
}

func rewriteNestedStmtForFrontend(stmt ast.Stmt, unused map[string]struct{}, hints *boolTypeHints, state *preprocessState) bool {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		return rewriteStmtListForFrontend(&s.List, unused, hints, state)
	case *ast.IfStmt:
		return rewriteStmtForFrontend(s, unused, hints, state)
	default:
		return rewriteStmtForFrontend(stmt, unused, hints, state)
	}
}

func rewriteExprForFrontend(expr ast.Expr, hints *boolTypeHints, state *preprocessState) (ast.Expr, bool) {
	if expr == nil {
		return nil, false
	}
	changed := false
	switch e := expr.(type) {
	case *ast.CallExpr:
		for i := range e.Args {
			arg, argChanged := rewriteExprForFrontend(e.Args[i], hints, state)
			e.Args[i] = arg
			changed = changed || argChanged
		}
		if helper, ok := boolConversionHelperName(e, hints); ok {
			state.requiredBoolHelpers[helper] = struct{}{}
			e.Fun = ast.NewIdent(helper)
			changed = true
		}
		return e, changed
	case *ast.BinaryExpr:
		x, xChanged := rewriteExprForFrontend(e.X, hints, state)
		y, yChanged := rewriteExprForFrontend(e.Y, hints, state)
		e.X = x
		e.Y = y
		return e, changed || xChanged || yChanged
	case *ast.UnaryExpr:
		x, xChanged := rewriteExprForFrontend(e.X, hints, state)
		e.X = x
		return e, changed || xChanged
	case *ast.ParenExpr:
		x, xChanged := rewriteExprForFrontend(e.X, hints, state)
		e.X = x
		return e, changed || xChanged
	case *ast.IndexExpr:
		x, xChanged := rewriteExprForFrontend(e.X, hints, state)
		idx, idxChanged := rewriteExprForFrontend(e.Index, hints, state)
		e.X = x
		e.Index = idx
		return e, changed || xChanged || idxChanged
	case *ast.SliceExpr:
		x, xChanged := rewriteExprForFrontend(e.X, hints, state)
		e.X = x
		return e, changed || xChanged
	case *ast.CompositeLit:
		for i := range e.Elts {
			elt, eltChanged := rewriteExprForFrontend(e.Elts[i], hints, state)
			e.Elts[i] = elt
			changed = changed || eltChanged
		}
		return e, changed
	default:
		return expr, false
	}
}

type boolTypeHints struct {
	scalars map[string]struct{}
	arrays  map[string]struct{}
}

func collectBoolTypeHints(fn *ast.FuncDecl) *boolTypeHints {
	hints := &boolTypeHints{
		scalars: make(map[string]struct{}),
		arrays:  make(map[string]struct{}),
	}
	if fn == nil {
		return hints
	}
	if fn.Type != nil && fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			switch classifyBoolType(field.Type) {
			case "scalar":
				for _, name := range field.Names {
					hints.scalars[name.Name] = struct{}{}
				}
			case "array":
				for _, name := range field.Names {
					hints.arrays[name.Name] = struct{}{}
				}
			}
		}
	}
	if fn.Body != nil {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.DeclStmt:
				gen, ok := v.Decl.(*ast.GenDecl)
				if !ok {
					return true
				}
				for _, spec := range gen.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					switch classifyBoolType(valueSpec.Type) {
					case "scalar":
						for _, name := range valueSpec.Names {
							hints.scalars[name.Name] = struct{}{}
						}
					case "array":
						for _, name := range valueSpec.Names {
							hints.arrays[name.Name] = struct{}{}
						}
					}
				}
			case *ast.AssignStmt:
				if v.Tok != token.DEFINE || len(v.Lhs) != len(v.Rhs) {
					return true
				}
				for i, lhs := range v.Lhs {
					name, ok := lhs.(*ast.Ident)
					if !ok || name.Name == "_" {
						continue
					}
					if exprIsDefinitelyBool(v.Rhs[i], hints) {
						hints.scalars[name.Name] = struct{}{}
					}
				}
			}
			return true
		})
	}
	return hints
}

func classifyBoolType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if t.Name == "bool" {
			return "scalar"
		}
	case *ast.ArrayType:
		if elt, ok := t.Elt.(*ast.Ident); ok && elt.Name == "bool" {
			return "array"
		}
	}
	return ""
}

func exprIsDefinitelyBool(expr ast.Expr, hints *boolTypeHints) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == "true" || e.Name == "false" {
			return true
		}
		_, ok := hints.scalars[e.Name]
		return ok
	case *ast.IndexExpr:
		id, ok := e.X.(*ast.Ident)
		if !ok {
			return false
		}
		_, ok = hints.arrays[id.Name]
		return ok
	case *ast.ParenExpr:
		return exprIsDefinitelyBool(e.X, hints)
	case *ast.UnaryExpr:
		return e.Op == token.NOT && exprIsDefinitelyBool(e.X, hints)
	case *ast.BinaryExpr:
		switch e.Op {
		case token.LAND, token.LOR:
			return exprIsDefinitelyBool(e.X, hints) && exprIsDefinitelyBool(e.Y, hints)
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func boolConversionHelperName(call *ast.CallExpr, hints *boolTypeHints) (string, bool) {
	if call == nil || len(call.Args) != 1 {
		return "", false
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", false
	}
	if !exprIsDefinitelyBool(call.Args[0], hints) {
		return "", false
	}
	switch fun.Name {
	case "uint8":
		return "mygoBoolToUint8", true
	case "uint16":
		return "mygoBoolToUint16", true
	case "uint32":
		return "mygoBoolToUint32", true
	case "uint64":
		return "mygoBoolToUint64", true
	case "uint":
		return "mygoBoolToUint", true
	case "int8":
		return "mygoBoolToInt8", true
	case "int16":
		return "mygoBoolToInt16", true
	case "int32":
		return "mygoBoolToInt32", true
	case "int64":
		return "mygoBoolToInt64", true
	case "int":
		return "mygoBoolToInt", true
	default:
		return "", false
	}
}

func collectUnusedLocalNames(fn *ast.FuncDecl) map[string]struct{} {
	defs := make(map[string]struct{})
	uses := make(map[string]struct{})
	if fn == nil || fn.Body == nil {
		return nil
	}
	collectStmtUses(fn.Body, defs, uses)
	unused := make(map[string]struct{})
	for name := range defs {
		if _, ok := uses[name]; !ok {
			unused[name] = struct{}{}
		}
	}
	return unused
}

func collectStmtUses(node ast.Node, defs, uses map[string]struct{}) {
	switch n := node.(type) {
	case *ast.BlockStmt:
		for _, stmt := range n.List {
			collectStmtUses(stmt, defs, uses)
		}
	case *ast.DeclStmt:
		gen, ok := n.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				if name.Name != "_" {
					defs[name.Name] = struct{}{}
				}
			}
			for _, value := range valueSpec.Values {
				collectExprUses(value, uses)
			}
		}
	case *ast.AssignStmt:
		if n.Tok == token.DEFINE {
			for _, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
					defs[id.Name] = struct{}{}
					continue
				}
				collectExprUses(lhs, uses)
			}
		} else {
			for _, lhs := range n.Lhs {
				switch lhs.(type) {
				case *ast.Ident:
					// Assignment targets do not count as uses for Go's unused-local rule.
				default:
					collectExprUses(lhs, uses)
				}
			}
		}
		for _, rhs := range n.Rhs {
			collectExprUses(rhs, uses)
		}
	case *ast.IfStmt:
		if n.Init != nil {
			collectStmtUses(n.Init, defs, uses)
		}
		collectExprUses(n.Cond, uses)
		collectStmtUses(n.Body, defs, uses)
		if n.Else != nil {
			collectStmtUses(n.Else, defs, uses)
		}
	case *ast.ForStmt:
		if n.Init != nil {
			collectStmtUses(n.Init, defs, uses)
		}
		if n.Cond != nil {
			collectExprUses(n.Cond, uses)
		}
		if n.Post != nil {
			collectStmtUses(n.Post, defs, uses)
		}
		collectStmtUses(n.Body, defs, uses)
	case *ast.SwitchStmt:
		if n.Init != nil {
			collectStmtUses(n.Init, defs, uses)
		}
		if n.Tag != nil {
			collectExprUses(n.Tag, uses)
		}
		for _, stmt := range n.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range cc.List {
				collectExprUses(expr, uses)
			}
			for _, bodyStmt := range cc.Body {
				collectStmtUses(bodyStmt, defs, uses)
			}
		}
	case *ast.ExprStmt:
		collectExprUses(n.X, uses)
	case *ast.ReturnStmt:
		for _, expr := range n.Results {
			collectExprUses(expr, uses)
		}
	case *ast.IncDecStmt:
		collectExprUses(n.X, uses)
	}
}

func collectExprUses(expr ast.Expr, uses map[string]struct{}) {
	ast.Inspect(expr, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		switch id.Name {
		case "_", "true", "false":
			return false
		}
		uses[id.Name] = struct{}{}
		return false
	})
}

func declaredNamesInStmt(stmt ast.Stmt) []string {
	var names []string
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok != token.DEFINE {
			return nil
		}
		for _, lhs := range s.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
				names = append(names, id.Name)
			}
		}
	case *ast.DeclStmt:
		gen, ok := s.Decl.(*ast.GenDecl)
		if !ok {
			return nil
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				if name.Name != "_" {
					names = append(names, name.Name)
				}
			}
		}
	}
	return names
}

func blankUseStmt(name string) ast.Stmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("_")},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{ast.NewIdent(name)},
	}
}

func addBoolHelpers(file *ast.File, state *preprocessState) {
	if file == nil || state == nil || len(state.requiredBoolHelpers) == 0 {
		return
	}
	helpers := make([]string, 0, len(state.requiredBoolHelpers))
	for name := range state.requiredBoolHelpers {
		helpers = append(helpers, name)
	}
	for _, name := range helpers {
		file.Decls = append(file.Decls, buildBoolHelperDecl(name))
	}
}

func buildBoolHelperDecl(name string) ast.Decl {
	params := []*ast.Field{{
		Names: []*ast.Ident{ast.NewIdent("v")},
		Type:  ast.NewIdent("bool"),
	}}
	body := []ast.Stmt{
		&ast.IfStmt{
			Cond: ast.NewIdent("v"),
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ReturnStmt{Results: []ast.Expr{
					&ast.BasicLit{Kind: token.INT, Value: "1"},
				}},
			}},
		},
		&ast.ReturnStmt{Results: []ast.Expr{
			&ast.BasicLit{Kind: token.INT, Value: "0"},
		}},
	}
	targetType := boolHelperTargetType(name)
	return &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: params},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: ast.NewIdent(targetType),
			}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}

func boolHelperTargetType(name string) string {
	switch name {
	case "mygoBoolToUint8":
		return "uint8"
	case "mygoBoolToUint16":
		return "uint16"
	case "mygoBoolToUint32":
		return "uint32"
	case "mygoBoolToUint64":
		return "uint64"
	case "mygoBoolToUint":
		return "uint"
	case "mygoBoolToInt8":
		return "int8"
	case "mygoBoolToInt16":
		return "int16"
	case "mygoBoolToInt32":
		return "int32"
	case "mygoBoolToInt64":
		return "int64"
	case "mygoBoolToInt":
		return "int"
	default:
		return "uint8"
	}
}
