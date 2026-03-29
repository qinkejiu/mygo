package stages

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	compareLoweringOptions = "locationInfoStyle=none,omitVersionComment"
	workloadsRoot          = "tests/stages"
)

type harness struct {
	repoRoot string
}

type testCase struct {
	Name       string
	Group      string
	SimCycles  int
	SimCompare simCompareMode
}

type simCompareMode string

const (
	simCompareStrict          simCompareMode = "strict"
	simCompareIgnoreLineOrder simCompareMode = "ignore-line-order"
)

var testCases = []testCase{
	{Name: "simple", Group: "scalar", SimCycles: 1, SimCompare: simCompareStrict},
	{Name: "simple_branch", Group: "control", SimCycles: 2, SimCompare: simCompareStrict},
	{Name: "simple_print", Group: "scalar", SimCycles: 1, SimCompare: simCompareStrict},
	{Name: "type_mismatch", Group: "scalar", SimCycles: 1, SimCompare: simCompareStrict},
	{Name: "comb_adder", Group: "comb", SimCycles: 1, SimCompare: simCompareStrict},
	{Name: "comb_bitwise", Group: "comb", SimCycles: 1, SimCompare: simCompareStrict},
	{Name: "comb_concat", Group: "comb", SimCycles: 1, SimCompare: simCompareStrict},
	{Name: "simple_channel", Group: "channels", SimCycles: 2, SimCompare: simCompareStrict},
	{Name: "phi_loop", Group: "control", SimCycles: 8, SimCompare: simCompareIgnoreLineOrder},
	{Name: "pipeline1", Group: "pipelines", SimCycles: 10, SimCompare: simCompareIgnoreLineOrder},
	{Name: "pipeline2", Group: "pipelines", SimCycles: 12, SimCompare: simCompareIgnoreLineOrder},
	{Name: "router_csp", Group: "channels", SimCycles: 16, SimCompare: simCompareIgnoreLineOrder},
}

var (
	circtOptAvailable  = checkBinary("circt-opt")
	verilatorAvailable = checkBinary("verilator")
	compareGoldens     = goldensEnabled()
)

func TestMLIRGeneration(t *testing.T) {
	requireGoldenValidation(t)
	runStageTests(t, func(t *testing.T, h harness, tc testCase) {
		if reason := unsupportedHardwareLoweringReason(tc); reason != "" {
			t.Skip(reason)
		}
		dir := filepath.Join(workloadsRoot, tc.Name)
		source := filepath.Join(dir, "main.go")
		mlirGolden := filepath.Join(dir, "main.mlir.golden")
		maybeVerifyMLIR(t, h.repoRoot, source, mlirGolden)
	})
}

func TestVerilogGeneration(t *testing.T) {
	requireGoldenValidation(t)
	runStageTests(t, func(t *testing.T, h harness, tc testCase) {
		if reason := unsupportedHardwareLoweringReason(tc); reason != "" {
			t.Skip(reason)
		}
		dir := filepath.Join(workloadsRoot, tc.Name)
		source := filepath.Join(dir, "main.go")
		verilogGolden := filepath.Join(dir, "main.sv.golden")
		maybeVerifyVerilog(t, h.repoRoot, source, verilogGolden)
	})
}

func TestSimulation(t *testing.T) {
	requireGoldenValidation(t)
	runStageTests(t, func(t *testing.T, h harness, tc testCase) {
		if reason := unsupportedHardwareLoweringReason(tc); reason != "" {
			t.Skip(reason)
		}
		dir := filepath.Join(workloadsRoot, tc.Name)
		source := filepath.Join(dir, "main.go")
		simGolden := filepath.Join(dir, "main.sim.golden")
		maybeVerifySimulation(t, h.repoRoot, source, simGolden, tc)
	})
}

func TestSimulationDetectsMismatch(t *testing.T) {
	if !circtOptAvailable {
		t.Skip("circt-opt not on PATH")
	}
	if !verilatorAvailable {
		t.Skip("verilator not on PATH")
	}
	h := newHarness(t)
	tc := getTestCase(t, "simple")
	if tc.SimCycles <= 0 {
		t.Skip("simple workload disables simulation")
	}
	dir := filepath.Join(workloadsRoot, tc.Name)
	source := filepath.Join(dir, "main.go")
	badExpect := filepath.Join(t.TempDir(), "bad.sim")
	if err := os.WriteFile(badExpect, []byte("mismatch\n"), 0o644); err != nil {
		t.Fatalf("write bad expect: %v", err)
	}
	args := []string{
		"run", "./cmd/mygo", "sim",
		"--sim-max-cycles", strconv.Itoa(tc.SimCycles),
		"--expect", badExpect,
		source,
	}
	output := runGoCommandExpectFailure(t, h.repoRoot, args...)
	if !strings.Contains(output, "simulator output mismatch") {
		t.Fatalf("unexpected sim mismatch output: %s", output)
	}
}

func TestSimulationVerilogOutWritesArtifacts(t *testing.T) {
	if !circtOptAvailable {
		t.Skip("circt-opt not on PATH")
	}
	if !verilatorAvailable {
		t.Skip("verilator not on PATH")
	}
	requireGoldenValidation(t)
	h := newHarness(t)
	tc := getTestCase(t, "simple")
	if tc.SimCycles <= 0 {
		t.Skip("simple workload disables simulation")
	}
	dir := filepath.Join(workloadsRoot, tc.Name)
	source := filepath.Join(dir, "main.go")
	simGolden := filepath.Join(dir, "main.sim.golden")
	if !fileExists(t, filepath.Join(h.repoRoot, simGolden)) {
		t.Skip("simple workload missing sim golden")
	}
	verilogOut := filepath.Join(t.TempDir(), "artifacts", "design.sv")
	args := []string{
		"run", "./cmd/mygo", "sim",
		"--sim-max-cycles", strconv.Itoa(tc.SimCycles),
		"--expect", simGolden,
		"--verilog-out", verilogOut,
		source,
	}
	runGoCommand(t, h.repoRoot, args...)
	info, err := os.Stat(verilogOut)
	if err != nil {
		t.Fatalf("verilog output missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("verilog output %s empty", verilogOut)
	}
}

func TestHardwareLoweringSupportsRouterCspMultiProducerChannel(t *testing.T) {
	if !circtOptAvailable {
		t.Skip("circt-opt not on PATH")
	}
	h := newHarness(t)
	source := filepath.Join(workloadsRoot, "router_csp", "main.go")
	output := filepath.Join(t.TempDir(), "router_csp.mlir")
	args := []string{"run", "./cmd/mygo", "compile", "-emit=mlir", "-o", output, source}
	runGoCommand(t, h.repoRoot, args...)
}

func TestDynamicLoopLowersToFSMVerilog(t *testing.T) {
	if !circtOptAvailable {
		t.Skip("circt-opt not on PATH")
	}
	h := newHarness(t)
	source := filepath.Join(workloadsRoot, "dynamic_loop_fsm", "main.go")
	output := filepath.Join(t.TempDir(), "dynamic_loop_fsm.sv")
	args := []string{
		"run", "./cmd/mygo", "compile",
		"-emit=verilog",
		"--circt-lowering-options", compareLoweringOptions,
		"-o", output,
		source,
	}
	runGoCommand(t, h.repoRoot, args...)

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read verilog output: %v", err)
	}
	top := extractTopModule(string(data))
	if strings.TrimSpace(top) == "" {
		t.Fatalf("missing top-level module in verilog output")
	}

	stateReg, ok := findStateRegisterName(top)
	if !ok {
		t.Fatalf("expected FSM state register in top module:\n%s", top)
	}
	if !strings.Contains(top, "case ("+stateReg) {
		t.Fatalf("expected FSM case statement for %q in top module:\n%s", stateReg, top)
	}
	if strings.Count(top, stateReg+" <=") < 2 {
		t.Fatalf("expected state transitions for %q in top module:\n%s", stateReg, top)
	}

	counter, ok := findSelfIncrementCounter(top)
	if !ok {
		t.Fatalf("expected loop counter self-increment in top module:\n%s", top)
	}
	if !strings.Contains(top, "reg [") || !strings.Contains(top, " "+counter+";") {
		t.Fatalf("expected loop counter register declaration for %q:\n%s", counter, top)
	}
	if !hasLoopBoundCompare(top, counter) {
		t.Fatalf("expected runtime loop bound compare using %q:\n%s", counter, top)
	}
	if !strings.Contains(top, "input_0") || !strings.Contains(top, "? input_") {
		t.Fatalf("expected datapath to depend on global input ports (not constant-folded):\n%s", top)
	}
}

func runStageTests(t *testing.T, fn func(*testing.T, harness, testCase)) {
	t.Helper()
	h := newHarness(t)

	grouped := make(map[string][]testCase)
	var groupOrder []string
	for _, tc := range testCases {
		group := tc.Group
		if group == "" {
			group = "ungrouped"
		}
		if _, ok := grouped[group]; !ok {
			groupOrder = append(groupOrder, group)
		}
		grouped[group] = append(grouped[group], tc)
	}

	for _, group := range groupOrder {
		group := group
		t.Run(group, func(t *testing.T) {
			for _, tc := range grouped[group] {
				tc := tc
				t.Run(tc.Name, func(t *testing.T) {
					t.Parallel()
					fn(t, h, tc)
				})
			}
		})
	}
}

func newHarness(t *testing.T) harness {
	t.Helper()
	repoRoot := determineRepoRoot(t)
	cacheDir := filepath.Join(repoRoot, ".gocache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("create go cache dir: %v", err)
	}
	t.Setenv("GOCACHE", cacheDir)
	return harness{repoRoot: repoRoot}
}

func maybeVerifyMLIR(t *testing.T, repoRoot, source, golden string) {
	t.Helper()
	if !fileExists(t, filepath.Join(repoRoot, golden)) {
		t.Fatalf("missing MLIR golden for %s: %s", source, golden)
	}
	output := filepath.Join(t.TempDir(), "main.mlir")
	args := []string{"run", "./cmd/mygo", "compile", "-emit=mlir", "-o", output, source}
	runGoCommand(t, repoRoot, args...)
	compareTextFiles(t, filepath.Join(repoRoot, golden), output)
}

func maybeVerifyVerilog(t *testing.T, repoRoot, source, golden string) {
	t.Helper()
	if !fileExists(t, filepath.Join(repoRoot, golden)) {
		t.Fatalf("missing Verilog golden for %s: %s", source, golden)
	}
	if !circtOptAvailable {
		t.Logf("skipping verilog check for %s: circt-opt not on PATH", source)
		return
	}
	output := filepath.Join(t.TempDir(), "main.sv")
	args := []string{
		"run", "./cmd/mygo", "compile",
		"-emit=verilog",
		"--circt-lowering-options", compareLoweringOptions,
		"-o", output,
	}
	args = append(args, source)
	runGoCommand(t, repoRoot, args...)
	compareTextFiles(t, filepath.Join(repoRoot, golden), output)
}

func maybeVerifySimulation(t *testing.T, repoRoot, source, golden string, tc testCase) {
	t.Helper()
	if tc.SimCycles <= 0 {
		t.Logf("skipping simulation for %s: sim cycles disabled", tc.Name)
		return
	}
	if !fileExists(t, filepath.Join(repoRoot, golden)) {
		t.Fatalf("missing simulation golden for %s: %s", source, golden)
		return
	}
	if !verilatorAvailable {
		t.Logf("skipping simulation for %s: verilator not on PATH", tc.Name)
		return
	}
	if !circtOptAvailable {
		t.Logf("skipping simulation for %s: circt-opt not on PATH", tc.Name)
		return
	}
	args := []string{
		"run", "./cmd/mygo", "sim",
		"--keep-artifacts=false",
		"--sim-max-cycles", strconv.Itoa(tc.SimCycles),
	}
	args = append(args, source)
	stdout, _ := runGoCommandCapture(t, repoRoot, args...)
	compareSimulationOutput(t, filepath.Join(repoRoot, golden), stdout, tc.SimCompare)
}

func runGoCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func runGoCommandCapture(t *testing.T, repoRoot string, args ...string) ([]byte, []byte) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.Bytes(), stderr.Bytes()
}

func runGoCommandExpectFailure(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("go %s succeeded unexpectedly", strings.Join(args, " "))
	}
	return string(out)
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		t.Fatalf("stat %s: %v", path, err)
	}
	return true
}

func compareTextFiles(t *testing.T, golden, actual string) {
	t.Helper()
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden %s: %v", golden, err)
	}
	got, err := os.ReadFile(actual)
	if err != nil {
		t.Fatalf("read actual %s: %v", actual, err)
	}
	if bytes.Equal(want, got) {
		return
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func compareSimulationOutput(t *testing.T, golden string, got []byte, mode simCompareMode) {
	t.Helper()
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read simulation golden %s: %v", golden, err)
	}
	if simulationOutputsMatch(want, got, mode) {
		return
	}
	t.Fatalf("simulation output mismatch for %s (mode=%s)\n%s", golden, mode, cmp.Diff(strings.TrimSpace(string(want)), strings.TrimSpace(string(got))))
}

func simulationOutputsMatch(want, got []byte, mode simCompareMode) bool {
	wantTrimmed := bytes.TrimSpace(want)
	gotTrimmed := bytes.TrimSpace(got)
	if bytes.Equal(wantTrimmed, gotTrimmed) {
		return true
	}
	if mode != simCompareIgnoreLineOrder {
		return false
	}
	wantLines := normalizedOutputLines(wantTrimmed)
	gotLines := normalizedOutputLines(gotTrimmed)
	if len(wantLines) != len(gotLines) {
		return false
	}
	wantSorted := append([]string(nil), wantLines...)
	gotSorted := append([]string(nil), gotLines...)
	sort.Strings(wantSorted)
	sort.Strings(gotSorted)
	return sameStringSlices(wantSorted, gotSorted)
}

func normalizedOutputLines(data []byte) []string {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func sameStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func getTestCase(t *testing.T, name string) testCase {
	t.Helper()
	for _, tc := range testCases {
		if tc.Name == name {
			return tc
		}
	}
	t.Fatalf("unknown test case %s", name)
	return testCase{}
}

func unsupportedHardwareLoweringReason(tc testCase) string {
	return ""
}

func checkBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func TestSimulationOutputsMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want string
		got  string
		mode simCompareMode
		ok   bool
	}{
		{
			name: "strict exact match",
			want: "a\nb\n",
			got:  "a\nb\n",
			mode: simCompareStrict,
			ok:   true,
		},
		{
			name: "strict rejects reordered lines",
			want: "a\nb\n",
			got:  "b\na\n",
			mode: simCompareStrict,
			ok:   false,
		},
		{
			name: "ignore line order accepts reordering",
			want: "producer sent 0\nconsumer received 0\nproducer sent 1\n",
			got:  "consumer received 0\nproducer sent 1\nproducer sent 0\n",
			mode: simCompareIgnoreLineOrder,
			ok:   true,
		},
		{
			name: "ignore line order still rejects content changes",
			want: "a\nb\n",
			got:  "a\nc\n",
			mode: simCompareIgnoreLineOrder,
			ok:   false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := simulationOutputsMatch([]byte(tc.want), []byte(tc.got), tc.mode); got != tc.ok {
				t.Fatalf("simulationOutputsMatch(%q, %q, %s)=%t, want %t", tc.want, tc.got, tc.mode, got, tc.ok)
			}
		})
	}
}

func requireGoldenValidation(t *testing.T) {
	t.Helper()
	if compareGoldens {
		return
	}
	t.Skip("artifact golden validation disabled; run MYGO_COMPARE_GOLDENS=1 go test ./... for full verification")
}

func goldensEnabled() bool {
	raw := os.Getenv("MYGO_COMPARE_GOLDENS")
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return enabled
}

func determineRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("determine repo root: %v", err)
	}
	return root
}

func extractTopModule(verilog string) string {
	start := strings.Index(verilog, "module main(")
	if start < 0 {
		return ""
	}
	rest := verilog[start:]
	end := strings.Index(rest, "\nendmodule")
	if end < 0 {
		return ""
	}
	return rest[:end+len("\nendmodule")]
}

func findStateRegisterName(moduleText string) (string, bool) {
	lines := strings.Split(moduleText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "reg [") || !strings.HasSuffix(line, ";") {
			continue
		}
		if !strings.Contains(line, " state_reg") {
			continue
		}
		fields := strings.Fields(strings.TrimSuffix(line, ";"))
		if len(fields) == 0 {
			continue
		}
		return fields[len(fields)-1], true
	}
	return "", false
}

func findSelfIncrementCounter(moduleText string) (string, bool) {
	lines := strings.Split(moduleText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "<=") || !strings.Contains(line, "+ 32'h1") {
			continue
		}
		parts := strings.SplitN(line, "<=", 2)
		if len(parts) != 2 {
			continue
		}
		lhs := strings.TrimSpace(parts[0])
		rhs := strings.TrimSpace(parts[1])
		rhsParts := strings.SplitN(rhs, "+ 32'h1", 2)
		if len(rhsParts) != 2 {
			continue
		}
		rhsBase := strings.TrimSpace(rhsParts[0])
		if lhs != "" && lhs == rhsBase {
			return lhs, true
		}
	}
	return "", false
}

func hasLoopBoundCompare(moduleText, counter string) bool {
	if counter == "" {
		return false
	}
	patterns := []string{
		"$signed(" + counter + ") < 32'sh8",
		counter + " < 32'sh8",
		counter + " < 8",
	}
	for _, p := range patterns {
		if strings.Contains(moduleText, p) {
			return true
		}
	}
	return false
}
