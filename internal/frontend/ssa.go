package frontend

import (
	"fmt"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"mygo/internal/diag"
)

// BuildSSA constructs SSA IR for the loaded packages.
func BuildSSA(pkgs []*packages.Package, reporter *diag.Reporter) (*ssa.Program, []*ssa.Package, error) {
	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no packages supplied for SSA construction")
	}

	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.SanityCheckFunctions)
	prog.Build()

	allNil := true
	for _, pkg := range ssaPkgs {
		if pkg != nil {
			allNil = false
			break
		}
	}
	if allNil {
		reporter.Errorf("SSA builder produced no packages (check for earlier errors)")
		return nil, nil, fmt.Errorf("ssa construction failed")
	}

	return prog, ssaPkgs, nil
}
