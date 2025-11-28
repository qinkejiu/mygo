package backend

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"mygo/internal/ir"
	"mygo/internal/mlir"
)

// Options configures how the CIRCT backend is invoked.
type Options struct {
	// CIRCTOptPath optionally overrides the circt-opt binary. When empty the
	// backend looks it up on PATH if needed.
	CIRCTOptPath string
	// CIRCTTranslatePath optionally overrides the circt-translate binary. When
	// empty the backend looks it up on PATH.
	CIRCTTranslatePath string
	// PassPipeline holds the circt-opt --pass-pipeline string (empty skips
	// circt-opt unless CIRCTOptPath is explicitly set).
	PassPipeline string
	// DumpMLIRPath writes the MLIR handed to CIRCT to the provided path when
	// non-empty.
	DumpMLIRPath string
	// KeepTemps preserves the intermediate directory on disk for debugging.
	KeepTemps bool
}

// EmitVerilog lowers the design to MLIR, optionally runs circt-opt, and invokes
// circt-translate --export-verilog to produce SystemVerilog at outputPath. When
// outputPath is empty or "-", stdout is used.
func EmitVerilog(design *ir.Design, outputPath string, opts Options) error {
	if design == nil {
		return fmt.Errorf("backend: design is nil")
	}

	translatePath, err := resolveBinary(opts.CIRCTTranslatePath, "circt-translate")
	if err != nil {
		return fmt.Errorf("backend: resolve circt-translate: %w", err)
	}

	var optPath string
	if needsOpt(opts) {
		if optPath, err = resolveBinary(opts.CIRCTOptPath, "circt-opt"); err != nil {
			return fmt.Errorf("backend: resolve circt-opt: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "mygo-circt-*")
	if err != nil {
		return fmt.Errorf("backend: create temp dir: %w", err)
	}
	if !opts.KeepTemps {
		defer os.RemoveAll(tempDir)
	}

	mlirPath := opts.DumpMLIRPath
	if mlirPath == "" {
		mlirPath = filepath.Join(tempDir, "design.mlir")
	} else if err := os.MkdirAll(filepath.Dir(mlirPath), 0o755); err != nil {
		return fmt.Errorf("backend: create circt-mlir dir: %w", err)
	}

	if err := mlir.Emit(design, mlirPath); err != nil {
		return fmt.Errorf("backend: emit mlir: %w", err)
	}

	currentInput := mlirPath
	if needsOpt(opts) {
		optOutput := filepath.Join(tempDir, "design.opt.mlir")
		if err := runCirctOpt(optPath, opts.PassPipeline, currentInput, optOutput); err != nil {
			return err
		}
		currentInput = optOutput
	}

	if err := runCirctTranslate(translatePath, currentInput, outputPath); err != nil {
		return err
	}

	return nil
}

func needsOpt(opts Options) bool {
	return opts.PassPipeline != "" || opts.CIRCTOptPath != ""
}

func runCirctOpt(binary, pipeline, inputPath, outputPath string) error {
	args := []string{inputPath, "-o", outputPath}
	if pipeline != "" {
		args = append(args, "--pass-pipeline="+pipeline)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backend: circt-opt failed: %w", err)
	}
	return nil
}

func runCirctTranslate(binary, inputPath, outputPath string) error {
	args := []string{"--export-verilog", inputPath}
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr

	writer, closer, err := outputWriter(outputPath)
	if err != nil {
		return err
	}
	defer closer()
	cmd.Stdout = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("backend: circt-translate failed: %w", err)
	}
	return nil
}

func outputWriter(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("backend: create output dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("backend: create output file: %w", err)
	}
	return f, f.Close, nil
}

func resolveBinary(explicit, fallback string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	path, err := exec.LookPath(fallback)
	if err != nil {
		return "", err
	}
	return path, nil
}
