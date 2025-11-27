package frontend

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	gopackages "golang.org/x/tools/go/packages"

	"mygo/internal/diag"
	llpackages "mygo/third_party/llgo/packages"
)

// LoadConfig configures how source files should be loaded before SSA
// translation. For Phase 1 we only need raw source filenames and optional
// build tags.
type LoadConfig struct {
	Sources   []string
	BuildTags []string
}

// LoadPackages loads the requested source files using LLGo's enhanced loader
// (see `third_party/llgo/tmp/README.md` under "Development tools") so we get
// the same caching/dedup behavior as their SSA pipeline.
func LoadPackages(cfg LoadConfig, reporter *diag.Reporter) ([]*gopackages.Package, *token.FileSet, error) {
	if len(cfg.Sources) == 0 {
		return nil, nil, fmt.Errorf("no source files were provided")
	}

	fset := token.NewFileSet()
	buildFlags := buildTagFlag(cfg.BuildTags)

	loadCfg := &gopackages.Config{
		Mode:  gopackages.NeedName | gopackages.NeedSyntax | gopackages.NeedFiles | gopackages.NeedCompiledGoFiles | gopackages.NeedTypes | gopackages.NeedTypesInfo | gopackages.NeedImports | gopackages.NeedDeps | gopackages.NeedModule | gopackages.NeedTypesSizes,
		Fset:  fset,
		Env:   append(os.Environ(), "GOOS=linux", "GOARCH=amd64"),
		Dir:   workingDir(cfg.Sources[0]),
		Tests: false,
	}

	if len(buildFlags) > 0 {
		loadCfg.BuildFlags = buildFlags
	}

	patterns, err := sourcePatterns(cfg.Sources)
	if err != nil {
		return nil, nil, err
	}

	deduper := llpackages.NewDeduper()
	pkgs, err := llpackages.LoadEx(deduper, nil, loadCfg, patterns...)
	if err != nil {
		return nil, nil, err
	}

	reporter.SetFileSet(fset)

	var hadErrors bool
	for _, pkg := range pkgs {
		for _, loadErr := range pkg.Errors {
			reporter.Errorf("%s: %s", loadErr.Pos, loadErr.Msg)
			hadErrors = true
		}
	}

	if hadErrors {
		return nil, nil, fmt.Errorf("package loading failed")
	}

	return pkgs, fset, nil
}

func sourcePatterns(sources []string) ([]string, error) {
	patterns := make([]string, 0, len(sources))
	for _, src := range sources {
		if src == "" {
			continue
		}
		abs, err := filepath.Abs(src)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", src, err)
		}
		patterns = append(patterns, "file="+abs)
	}
	if len(patterns) == 0 {
		return nil, fmt.Errorf("no valid source files provided")
	}
	return patterns, nil
}

func buildTagFlag(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	joined := strings.Join(tags, ",")
	if joined == "" {
		return nil
	}
	return []string{"-tags=" + joined}
}

func workingDir(sample string) string {
	if sample == "" {
		return ""
	}
	dir := filepath.Dir(sample)
	if dir == "." {
		return ""
	}
	return dir
}
