package chstone

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

type testCase struct {
	Name      string
	SimCycles int
}

var (
	circtOptAvailable  = checkBinary("circt-opt")
	verilatorAvailable = checkBinary("verilator")
)

var testCases = []testCase{
	{Name: "aes", SimCycles: 512},
	{Name: "dfsin", SimCycles: 8192},
	{Name: "sha", SimCycles: 8},
}

func TestHardwareMatchesSoftware(t *testing.T) {
	requireGoldenValidation(t)
	if !circtOptAvailable {
		t.Skip("circt-opt not on PATH")
	}
	if !verilatorAvailable {
		t.Skip("verilator not on PATH")
	}

	repoRoot := determineRepoRoot(t)
	cacheDir := filepath.Join(repoRoot, ".gocache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("create go cache dir: %v", err)
	}
	t.Setenv("GOCACHE", cacheDir)

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			source := filepath.Join("tests", "CHStone", tc.Name, "main.go")
			softwareStdout, softwareStderr := runGoCommandCapture(t, repoRoot, "run", source)
			hardwareStdout, hardwareStderr := runGoCommandCapture(
				t,
				repoRoot,
				"run", "./cmd/mygo", "sim",
				"--keep-artifacts=false",
				"--sim-max-cycles", strconv.Itoa(tc.SimCycles),
				source,
			)
			if diff := cmp.Diff(
				strings.TrimSpace(string(normalizeSimulatorStdout(softwareStdout))),
				strings.TrimSpace(string(normalizeSimulatorStdout(hardwareStdout))),
			); diff != "" {
				t.Fatalf(
					"hardware/software stdout mismatch for %s (-software +hardware):\n%s\nsoftware stderr:\n%s\nhardware stderr:\n%s",
					tc.Name,
					diff,
					string(softwareStderr),
					string(hardwareStderr),
				)
			}
		})
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

func normalizeSimulatorStdout(data []byte) []byte {
	replacer := strings.NewReplacer(
		"(nan)", "(NaN)",
		"(-nan)", "(NaN)",
		"(inf)", "(+Inf)",
		"(-inf)", "(-Inf)",
	)
	text := replacer.Replace(string(data))
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = normalizeHexByteRunLine(line)
	}
	return []byte(strings.Join(lines, "\n"))
}

func normalizeHexByteRunLine(line string) string {
	tab := strings.IndexByte(line, '\t')
	if tab < 0 || tab+1 >= len(line) {
		return line
	}
	body := strings.TrimSpace(line[tab+1:])
	if body == "" {
		return line
	}
	for _, ch := range body {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return line
		}
	}
	normalized, ok := decodeZeroPaddedHexBytes(body)
	if !ok {
		return line
	}
	return line[:tab+1] + normalized
}

func decodeZeroPaddedHexBytes(body string) (string, bool) {
	if len(body) < 8 {
		return "", false
	}
	var out strings.Builder
	for i := 0; i < len(body); {
		switch {
		case i+9 <= len(body) && body[i:i+8] == "00000000":
			out.WriteByte('0')
			out.WriteByte(asciiLowerHex(body[i+8]))
			i += 9
		case i+8 <= len(body) && body[i:i+6] == "000000":
			out.WriteByte(asciiLowerHex(body[i+6]))
			out.WriteByte(asciiLowerHex(body[i+7]))
			i += 8
		default:
			return "", false
		}
	}
	return out.String(), true
}

func asciiLowerHex(ch byte) byte {
	if ch >= 'A' && ch <= 'F' {
		return ch + ('a' - 'A')
	}
	return ch
}

func requireGoldenValidation(t *testing.T) {
	t.Helper()
	if goldensEnabled() {
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

func checkBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func determineRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("determine repo root: %v", err)
	}
	return root
}
