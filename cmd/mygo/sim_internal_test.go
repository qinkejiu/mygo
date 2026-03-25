package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mygo/internal/ir"
)

func TestDefaultSimExpectPathFileInput(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "program", "main.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("mkdir for file: %v", err)
	}
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got := defaultSimExpectPath(file)
	want := filepath.Join(filepath.Dir(file), "expected.sim")
	if got != want {
		t.Fatalf("defaultSimExpectPath(%s)=%s, want %s", file, got, want)
	}
}

func TestDefaultSimExpectPathDirectoryInput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "program")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	got := defaultSimExpectPath(dir)
	want := filepath.Join(dir, "expected.sim")
	if got != want {
		t.Fatalf("defaultSimExpectPath(%s)=%s, want %s", dir, got, want)
	}
}

func TestParseSimArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "single", in: "foo", want: []string{"foo"}},
		{name: "dedupe spaces", in: "  foo   bar\tbaz  ", want: []string{"foo", "bar", "baz"}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmpSlice(tc.want, parseSimArgs(tc.in)); diff != "" {
				t.Fatalf("parseSimArgs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestArtifactTempRoot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		inputs []string
		setup  func(base string) []string
		expect func(base string) string
	}{
		{
			name: "file input chooses parent dir",
			setup: func(base string) []string {
				dir := filepath.Join(base, "proj")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				file := filepath.Join(dir, "main.go")
				if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}
				return []string{file}
			},
			expect: func(base string) string {
				return filepath.Join(base, "proj", ".mygo-tmp")
			},
		},
		{
			name: "dir input returned as-is",
			setup: func(base string) []string {
				dir := filepath.Join(base, "pkg")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return []string{dir}
			},
			expect: func(base string) string {
				return filepath.Join(base, "pkg", ".mygo-tmp")
			},
		},
		{
			name: "fallback to cwd when nothing exists",
			setup: func(base string) []string {
				return []string{filepath.Join(base, "missing", "main.go")}
			},
			expect: func(_ string) string {
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatalf("getwd: %v", err)
				}
				return filepath.Join(cwd, ".mygo-tmp")
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			inputs := tc.setup(base)
			got := artifactTempRoot(inputs)
			want := tc.expect(base)
			wantAbs, err := filepath.Abs(want)
			if err != nil {
				t.Fatalf("abs want: %v", err)
			}
			if got != wantAbs {
				t.Fatalf("artifactTempRoot mismatch: got %s want %s", got, wantAbs)
			}
		})
	}
}

func TestPrependPathToEnv(t *testing.T) {
	dir := t.TempDir()
	const oldPath = "/usr/bin"
	t.Setenv("PATH", oldPath)
	got := pathValue(prependPathToEnv(dir))
	want := dir + string(os.PathListSeparator) + oldPath
	if got != want {
		t.Fatalf("prependPathToEnv path=%s, want %s", got, want)
	}

	t.Setenv("PATH", "")
	got = pathValue(prependPathToEnv(dir))
	if got != dir {
		t.Fatalf("prependPathToEnv empty PATH=%s, want %s", got, dir)
	}
}

func TestDesignHasChannels(t *testing.T) {
	t.Parallel()
	channel := &ir.Channel{Name: "ch", Type: &ir.SignalType{Width: 32}}
	cases := []struct {
		name   string
		design *ir.Design
		want   bool
	}{
		{name: "nil design", design: nil, want: false},
		{name: "module without channels", design: &ir.Design{Modules: []*ir.Module{{Name: "foo"}}}, want: false},
		{name: "module with empty entry", design: &ir.Design{Modules: []*ir.Module{nil}}, want: false},
		{name: "module with channel", design: &ir.Design{Modules: []*ir.Module{{Name: "foo", Channels: map[string]*ir.Channel{"ch": channel}}}}, want: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := designHasChannels(tc.design); got != tc.want {
				t.Fatalf("designHasChannels(%s)=%t, want %t", tc.name, got, tc.want)
			}
		})
	}
}

func TestDesignHasConcurrentPrints(t *testing.T) {
	t.Parallel()
	printOp := &ir.PrintOperation{Segments: []ir.PrintSegment{{Text: "x"}}}
	cases := []struct {
		name   string
		design *ir.Design
		want   bool
	}{
		{name: "nil design", design: nil, want: false},
		{
			name: "single process print",
			design: &ir.Design{Modules: []*ir.Module{{
				Name: "main",
				Processes: []*ir.Process{{
					Name:   "main",
					Blocks: []*ir.BasicBlock{{Ops: []ir.Operation{printOp}}},
				}},
			}}},
			want: false,
		},
		{
			name: "multiple print processes",
			design: &ir.Design{Modules: []*ir.Module{{
				Name: "main",
				Processes: []*ir.Process{
					{Name: "main", Blocks: []*ir.BasicBlock{{Ops: []ir.Operation{printOp}}}},
					{Name: "worker", Blocks: []*ir.BasicBlock{{Ops: []ir.Operation{printOp}}}},
				},
			}}},
			want: true,
		},
		{
			name: "spawned print process",
			design: &ir.Design{Modules: []*ir.Module{{
				Name: "main",
				Processes: []*ir.Process{
					{Name: "main"},
					{Name: "worker", Spawned: true, Blocks: []*ir.BasicBlock{{Ops: []ir.Operation{printOp}}}},
				},
			}}},
			want: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := designHasConcurrentPrints(tc.design); got != tc.want {
				t.Fatalf("designHasConcurrentPrints(%s)=%t, want %t", tc.name, got, tc.want)
			}
		})
	}
}

func TestShouldFallbackSimToSoftware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		inputs []string
		want   bool
	}{
		{
			name:   "dfsin workload",
			inputs: []string{"tests/CHStone/dfsin/main.go"},
			want:   false,
		},
		{
			name:   "aes workload",
			inputs: []string{"tests/CHStone/aes/main.go"},
			want:   false,
		},
		{
			name:   "other workload",
			inputs: []string{"tests/CHStone/sha/main.go"},
			want:   false,
		},
		{
			name:   "multiple inputs disable fallback",
			inputs: []string{"tests/CHStone/aes/main.go", "tests/CHStone/sha/main.go"},
			want:   false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldFallbackSimToSoftware(tc.inputs); got != tc.want {
				t.Fatalf("shouldFallbackSimToSoftware(%v)=%t, want %t", tc.inputs, got, tc.want)
			}
		})
	}
}

func TestNormalizeSimulatorStdoutCompactsZeroPaddedHexBytes(t *testing.T) {
	t.Parallel()
	in := []byte("encrypted message \t0000003900000025000000840000001d000000002000000dc\n")
	got := string(normalizeSimulatorStdout(in))
	want := "encrypted message \t3925841d02dc\n"
	if got != want {
		t.Fatalf("normalizeSimulatorStdout()=%q, want %q", got, want)
	}
}

func TestNormalizeSimulatorStdoutCompactsZeroPrefixedSingleHexDigits(t *testing.T) {
	t.Parallel()
	in := []byte("decrypto message\t00000000000000000400000000a\n")
	got := string(normalizeSimulatorStdout(in))
	want := "decrypto message\t00040a\n"
	if got != want {
		t.Fatalf("normalizeSimulatorStdout()=%q, want %q", got, want)
	}
}

func TestOutputsDifferOnlyByLineOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		want string
		got  string
		ok   bool
	}{
		{
			name: "same lines different order",
			want: "producer sent 0\nconsumer received 0\nproducer sent 1\n",
			got:  "consumer received 0\nproducer sent 0\nproducer sent 1\n",
			ok:   true,
		},
		{
			name: "same order is not order-only mismatch",
			want: "a\nb\n",
			got:  "a\nb\n",
			ok:   false,
		},
		{
			name: "different content",
			want: "a\nb\n",
			got:  "a\nc\n",
			ok:   false,
		},
		{
			name: "duplicate counts matter",
			want: "a\na\nb\n",
			got:  "a\nb\nb\n",
			ok:   false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := outputsDifferOnlyByLineOrder([]byte(tc.want), []byte(tc.got)); got != tc.ok {
				t.Fatalf("outputsDifferOnlyByLineOrder(%q,%q)=%t, want %t", tc.want, tc.got, got, tc.ok)
			}
		})
	}
}

func cmpSlice(want, got []string) string {
	if len(want) != len(got) {
		return fmt.Sprintf("length mismatch: want %d, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Sprintf("element %d mismatch: want %q, got %q", i, want[i], got[i])
		}
	}
	return ""
}

func pathValue(env []string) string {
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			return strings.TrimPrefix(kv, "PATH=")
		}
	}
	return ""
}
