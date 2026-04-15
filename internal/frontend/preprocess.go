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

	"golang.org/x/tools/go/ast/astutil"
)

type preprocessState struct {
	requiredBoolHelpers map[string]struct{}
}

const staticLoopUnrollLimit = 2048

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
		typeHints := collectBoolTypeHints(fn)
		unused := collectUnusedLocalNames(fn)
		bodyChanged := rewriteStmtListForFrontend(&fn.Body.List, unused, typeHints, state)
		changed = changed || fnChanged || bodyChanged
	}
	return changed, nil
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
	for _, stmt := range *list {
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
	switch assign.Tok {
	case token.DEFINE, token.ASSIGN:
	default:
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
	targetType := boolHelperTargetType(name)
	return &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent("v")},
				Type:  ast.NewIdent("bool"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: ast.NewIdent(targetType),
			}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
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
		}},
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
