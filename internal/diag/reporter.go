package diag

import (
	"fmt"
	"go/token"
	"io"
	"os"
)

// Severity represents the level of a diagnostic entry.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

// Reporter collects and prints diagnostics in either text or JSON format.
// For Phase 1 we only implement a lightweight text reporter.
type Reporter struct {
	out      io.Writer
	format   string
	fset     *token.FileSet
	errCount int
}

// NewReporter creates a reporter that writes to out using the given format.
func NewReporter(out io.Writer, format string) *Reporter {
	if out == nil {
		out = os.Stderr
	}
	if format == "" {
		format = "text"
	}
	return &Reporter{
		out:    out,
		format: format,
		fset:   token.NewFileSet(),
	}
}

// SetFileSet configures the file set used to turn token.Pos values into
// user-friendly locations.
func (r *Reporter) SetFileSet(fset *token.FileSet) {
	if fset != nil {
		r.fset = fset
	}
}

// Info reports an informational message.
func (r *Reporter) Info(pos token.Pos, msg string) {
	r.report(Info, pos, msg)
}

// Warning reports a warning message.
func (r *Reporter) Warning(pos token.Pos, msg string) {
	r.report(Warning, pos, msg)
}

// Error reports an error message and increases the error count.
func (r *Reporter) Error(pos token.Pos, msg string) {
	r.report(Error, pos, msg)
	r.errCount++
}

// Errorf reports an error without an explicit token position.
func (r *Reporter) Errorf(format string, args ...any) {
	r.Error(token.NoPos, fmt.Sprintf(format, args...))
}

// HasErrors indicates whether an error has been reported.
func (r *Reporter) HasErrors() bool {
	return r.errCount > 0
}

func (r *Reporter) report(sev Severity, pos token.Pos, msg string) {
	if r.format != "text" {
		// Only text output is implemented for now.
	}

	location := ""
	if pos != token.NoPos && r.fset != nil {
		p := r.fset.Position(pos)
		if p.IsValid() {
			location = fmt.Sprintf("%s:%d:%d: ", p.Filename, p.Line, p.Column)
		}
	}

	var prefix string
	switch sev {
	case Info:
		prefix = "info"
	case Warning:
		prefix = "warning"
	default:
		prefix = "error"
	}

	fmt.Fprintf(r.out, "%s%s: %s\n", location, prefix, msg)
}
