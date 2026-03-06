package stages

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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
	fifoLib  string
}

type testCase struct {
	Name      string
	Group     string
	NeedsFIFO bool
	SimCycles int
}

var testCases = []testCase{
	{Name: "simple", Group: "scalar", SimCycles: 1},
	{Name: "simple_branch", Group: "control", SimCycles: 2},
	{Name: "simple_print", Group: "scalar", SimCycles: 1},
	{Name: "type_mismatch", Group: "scalar", SimCycles: 1},
	{Name: "comb_adder", Group: "comb", SimCycles: 1},
	{Name: "comb_bitwise", Group: "comb", SimCycles: 1},
	{Name: "comb_concat", Group: "comb", SimCycles: 1},
	{Name: "simple_channel", Group: "channels", NeedsFIFO: true, SimCycles: 2},
	{Name: "phi_loop", Group: "control", NeedsFIFO: true, SimCycles: 8},
	{Name: "pipeline1", Group: "pipelines", NeedsFIFO: true, SimCycles: 10},
	{Name: "pipeline2", Group: "pipelines", NeedsFIFO: true, SimCycles: 12},
	{Name: "router_csp", Group: "channels", NeedsFIFO: true, SimCycles: 16},
}

var (
	circtOptAvailable  = checkBinary("circt-opt")
	verilatorAvailable = checkBinary("verilator")
	compareGoldens     = goldensEnabled()
)

func TestMLIRGeneration(t *testing.T) {
	runStageTests(t, func(t *testing.T, h harness, tc testCase) {
		dir := filepath.Join(workloadsRoot, tc.Name)
		source := filepath.Join(dir, "main.go")
		mlirGolden := filepath.Join(dir, "main.mlir.golden")
		maybeVerifyMLIR(t, h.repoRoot, source, mlirGolden)
	})
}

func TestVerilogGeneration(t *testing.T) {
	runStageTests(t, func(t *testing.T, h harness, tc testCase) {
		dir := filepath.Join(workloadsRoot, tc.Name)
		source := filepath.Join(dir, "main.go")
		verilogGolden := filepath.Join(dir, "main.sv.golden")
		maybeVerifyVerilog(t, h.repoRoot, source, verilogGolden, h.fifoLib, tc.NeedsFIFO)
	})
}

func TestSimulation(t *testing.T) {
	runStageTests(t, func(t *testing.T, h harness, tc testCase) {
		dir := filepath.Join(workloadsRoot, tc.Name)
		source := filepath.Join(dir, "main.go")
		simGolden := filepath.Join(dir, "main.sim.golden")
		maybeVerifySimulation(t, h.repoRoot, source, simGolden, h.fifoLib, tc)
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
	if !compareGoldens {
		t.Skip("golden comparison disabled (set MYGO_COMPARE_GOLDENS=1 to re-enable)")
	}
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
	fifoLib := filepath.Join(repoRoot, "internal", "backend", "templates", "simple_fifo.sv")
	cacheDir := filepath.Join(repoRoot, ".gocache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("create go cache dir: %v", err)
	}
	t.Setenv("GOCACHE", cacheDir)
	return harness{repoRoot: repoRoot, fifoLib: fifoLib}
}

func maybeVerifyMLIR(t *testing.T, repoRoot, source, golden string) {
	t.Helper()
	if !compareGoldens {
		t.Logf("skipping MLIR golden for %s: MYGO_COMPARE_GOLDENS not enabled", source)
		return
	}
	if !fileExists(t, filepath.Join(repoRoot, golden)) {
		return
	}
	output := filepath.Join(t.TempDir(), "main.mlir")
	args := []string{"run", "./cmd/mygo", "compile", "-emit=mlir", "-o", output, source}
	runGoCommand(t, repoRoot, args...)
	compareTextFiles(t, filepath.Join(repoRoot, golden), output)
}

func maybeVerifyVerilog(t *testing.T, repoRoot, source, golden, fifoLib string, needsFIFO bool) {
	t.Helper()
	if !compareGoldens {
		t.Logf("skipping Verilog golden for %s: MYGO_COMPARE_GOLDENS not enabled", source)
		return
	}
	if !fileExists(t, filepath.Join(repoRoot, golden)) {
		return
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
	if needsFIFO {
		args = append(args, "--fifo-src", fifoLib)
	}
	args = append(args, source)
	runGoCommand(t, repoRoot, args...)
	compareTextFiles(t, filepath.Join(repoRoot, golden), output)
}

func maybeVerifySimulation(t *testing.T, repoRoot, source, golden, fifoLib string, tc testCase) {
	t.Helper()
	if !compareGoldens {
		t.Logf("skipping simulation golden for %s: MYGO_COMPARE_GOLDENS not enabled", source)
		return
	}
	if !fileExists(t, filepath.Join(repoRoot, golden)) || tc.SimCycles <= 0 {
		if tc.SimCycles <= 0 {
			t.Logf("skipping simulation for %s: sim cycles disabled", tc.Name)
		}
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
		"--sim-max-cycles", strconv.Itoa(tc.SimCycles),
		"--expect", golden,
	}
	if tc.NeedsFIFO {
		args = append(args, "--fifo-src", fifoLib)
	}
	args = append(args, source)
	runGoCommand(t, repoRoot, args...)
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

func checkBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
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
