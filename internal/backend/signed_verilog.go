package backend

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"mygo/internal/ir"
)

type printOperandInfo struct {
	signed bool
	width  int
}

type printInfo struct {
	operands []printOperandInfo
}

var (
	verilogLiteralRe = regexp.MustCompile(`\b(\d+)'([bBdDhH])([0-9a-fA-F_xXzZ?]+)\b`)
	declRe           = regexp.MustCompile(`^(\s*)(wire|reg|inout|input|output)\s+(signed\s+)?(\[[^\]]+\]\s+)?([A-Za-z_][A-Za-z0-9_$]*)\b(.*)$`)
	assignRe         = regexp.MustCompile(`^(\s*)assign\s+([A-Za-z_][A-Za-z0-9_$]*)\s*=\s*(.*?);\s*$`)
)

func applySignedVerilog(design *ir.Design, verilogPath string) error {
	if design == nil || verilogPath == "" {
		return nil
	}
	data, err := os.ReadFile(verilogPath)
	if err != nil {
		return fmt.Errorf("backend: read verilog output: %w", err)
	}
	src := string(data)
	prints := collectPrintInfos(design)
	signedNames := collectSignedNames(design)

	rewritten, printNames, err := rewriteFwriteCalls(src, prints)
	if err != nil {
		return err
	}
	for name := range printNames {
		signedNames[name] = struct{}{}
	}
	rewritten = rewriteSignedDeclsAndAssigns(rewritten, signedNames)

	if rewritten != src {
		if err := os.WriteFile(verilogPath, []byte(rewritten), 0o644); err != nil {
			return fmt.Errorf("backend: update verilog signedness: %w", err)
		}
	}
	return nil
}

func collectPrintInfos(design *ir.Design) []printInfo {
	if design == nil {
		return nil
	}
	var prints []printInfo
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		ordered := orderProcesses(module)
		for _, proc := range ordered {
			if proc == nil {
				continue
			}
			for _, block := range proc.Blocks {
				for _, op := range block.Ops {
					p, ok := op.(*ir.PrintOperation)
					if !ok || p == nil {
						continue
					}
					info := printInfo{operands: make([]printOperandInfo, 0, len(p.Segments))}
					for _, seg := range p.Segments {
						if seg.Value == nil || seg.Value.Type == nil {
							continue
						}
						width := seg.Value.Type.Width
						if width <= 0 {
							width = 1
						}
						info.operands = append(info.operands, printOperandInfo{
							signed: seg.Value.Type.Signed && width > 1,
							width:  width,
						})
					}
					prints = append(prints, info)
				}
			}
		}
	}
	return prints
}

func collectSignedNames(design *ir.Design) map[string]struct{} {
	signed := make(map[string]struct{})
	if design == nil {
		return signed
	}
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		for _, port := range module.Ports {
			if port.Type == nil || !port.Type.Signed || port.Type.Width <= 1 {
				continue
			}
			signed[sanitize(port.Name)] = struct{}{}
		}
		for _, sig := range module.Signals {
			if sig == nil || sig.Type == nil || !sig.Type.Signed || sig.Type.Width <= 1 {
				continue
			}
			signed[sanitize(sig.Name)] = struct{}{}
		}
		for _, ch := range module.Channels {
			if ch == nil || ch.Type == nil || !ch.Type.Signed || ch.Type.Width <= 1 {
				continue
			}
			name := sanitize(ch.Name)
			signed["chan_"+name+"_wdata"] = struct{}{}
			signed["chan_"+name+"_rdata"] = struct{}{}
		}
	}
	return signed
}

func orderProcesses(module *ir.Module) []*ir.Process {
	if module == nil {
		return nil
	}
	type procInfo struct {
		proc       *ir.Process
		moduleName string
	}
	infos := make([]procInfo, 0, len(module.Processes))
	for _, proc := range module.Processes {
		if proc == nil {
			continue
		}
		infos = append(infos, procInfo{proc: proc, moduleName: processModuleName(module, proc)})
	}
	sort.SliceStable(infos, func(i, j int) bool { return infos[i].moduleName < infos[j].moduleName })

	var root *ir.Process
	ordered := make([]*ir.Process, 0, len(infos))
	for _, info := range infos {
		if info.proc != nil && info.proc.Name == module.Name && root == nil {
			root = info.proc
			continue
		}
		ordered = append(ordered, info.proc)
	}
	if root != nil {
		ordered = append([]*ir.Process{root}, ordered...)
	}
	return ordered
}

func processModuleName(module *ir.Module, proc *ir.Process) string {
	modName := "module"
	if module != nil && module.Name != "" {
		modName = sanitize(module.Name)
	}
	if proc == nil {
		return modName
	}
	if proc.Name != "" {
		return modName + "__proc_" + sanitize(proc.Name)
	}
	return modName + "__proc"
}

func rewriteFwriteCalls(src string, prints []printInfo) (string, map[string]struct{}, error) {
	signedNames := make(map[string]struct{})
	var out strings.Builder
	idx := 0
	i := 0
	for i < len(src) {
		start := strings.Index(src[i:], "$fwrite")
		if start == -1 {
			out.WriteString(src[i:])
			break
		}
		start += i
		out.WriteString(src[i:start])
		j := start + len("$fwrite")
		for j < len(src) && unicode.IsSpace(rune(src[j])) {
			j++
		}
		if j >= len(src) || src[j] != '(' {
			out.WriteString(src[start : start+len("$fwrite")])
			i = start + len("$fwrite")
			continue
		}
		end, err := findMatchingParen(src, j)
		if err != nil {
			return "", nil, err
		}
		argsStr := src[j+1 : end]
		args, seps, err := splitArgsWithSeps(argsStr)
		if err != nil {
			return "", nil, err
		}
		if idx < len(prints) {
			args, signedNames = rewriteFwriteArgs(args, prints[idx], signedNames)
			idx++
		}
		out.WriteString("$fwrite(")
		for k, arg := range args {
			if k > 0 {
				out.WriteString(seps[k-1])
			}
			out.WriteString(arg)
		}
		out.WriteString(")")
		i = end + 1
	}
	return out.String(), signedNames, nil
}

func rewriteFwriteArgs(args []string, info printInfo, signedNames map[string]struct{}) ([]string, map[string]struct{}) {
	if signedNames == nil {
		signedNames = make(map[string]struct{})
	}
	if len(args) < 2 {
		return args, signedNames
	}
	for i := 2; i < len(args); i++ {
		opIdx := i - 2
		if opIdx >= len(info.operands) {
			break
		}
		operand := info.operands[opIdx]
		if !operand.signed {
			continue
		}
		raw := args[i]
		leading, core, trailing := trimArg(raw)
		if core == "" {
			continue
		}
		switch {
		case isNumericLiteral(core):
			core = makeLiteralSigned(core)
		case isIdentifier(core):
			signedNames[core] = struct{}{}
		default:
			core = "$signed(" + core + ")"
		}
		args[i] = leading + core + trailing
	}
	return args, signedNames
}

func rewriteSignedDeclsAndAssigns(src string, signedNames map[string]struct{}) string {
	if len(signedNames) == 0 {
		return src
	}
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if matches := declRe.FindStringSubmatch(line); matches != nil {
			name := matches[5]
			if _, ok := signedNames[name]; ok && matches[3] == "" && matches[4] != "" {
				line = matches[1] + matches[2] + " signed " + matches[4] + name + matches[6]
			}
			if _, ok := signedNames[name]; ok {
				if eq := strings.Index(line, "="); eq != -1 {
					before := line[:eq+1]
					after := rewriteSignedLiterals(line[eq+1:])
					line = before + after
				}
			}
			lines[i] = line
			continue
		}
		if matches := assignRe.FindStringSubmatch(line); matches != nil {
			name := matches[2]
			if _, ok := signedNames[name]; ok {
				expr := rewriteSignedLiterals(matches[3])
				line = matches[1] + "assign " + name + " = " + expr + ";"
				lines[i] = line
			}
		}
	}
	return strings.Join(lines, "\n")
}

func splitArgsWithSeps(argsStr string) ([]string, []string, error) {
	var args []string
	var seps []string
	start := 0
	depthParen, depthBrace, depthBracket := 0, 0, 0
	inString := false
	inLineComment := false
	inBlockComment := false
	for i := 0; i < len(argsStr); i++ {
		ch := argsStr[i]
		next := byte(0)
		if i+1 < len(argsStr) {
			next = argsStr[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if ch == '\\' {
				i++
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && next == '/' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case ',':
			if depthParen == 0 && depthBrace == 0 && depthBracket == 0 {
				args = append(args, argsStr[start:i])
				j := i + 1
				for j < len(argsStr) && unicode.IsSpace(rune(argsStr[j])) {
					j++
				}
				seps = append(seps, argsStr[i:j])
				start = j
			}
		}
	}
	args = append(args, argsStr[start:])
	return args, seps, nil
}

func trimArg(raw string) (leading, core, trailing string) {
	start := 0
	end := len(raw)
	for start < end && unicode.IsSpace(rune(raw[start])) {
		start++
	}
	for end > start && unicode.IsSpace(rune(raw[end-1])) {
		end--
	}
	leading = raw[:start]
	core = raw[start:end]
	trailing = raw[end:]
	return
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	r := rune(s[0])
	if !(r == '_' || unicode.IsLetter(r)) {
		return false
	}
	for i := 1; i < len(s); i++ {
		r = rune(s[i])
		if !(r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func isNumericLiteral(s string) bool {
	return verilogLiteralRe.MatchString(s)
}

func makeLiteralSigned(s string) string {
	if !verilogLiteralRe.MatchString(s) || strings.Contains(s, "'s") || strings.Contains(s, "'S") {
		return s
	}
	parts := verilogLiteralRe.FindStringSubmatch(s)
	if len(parts) != 4 {
		return s
	}
	return parts[1] + "'s" + parts[2] + parts[3]
}

func rewriteSignedLiterals(expr string) string {
	return verilogLiteralRe.ReplaceAllStringFunc(expr, func(m string) string {
		if strings.Contains(m, "'s") || strings.Contains(m, "'S") {
			return m
		}
		parts := verilogLiteralRe.FindStringSubmatch(m)
		if len(parts) != 4 {
			return m
		}
		return parts[1] + "'s" + parts[2] + parts[3]
	})
}

func findMatchingParen(src string, openIdx int) (int, error) {
	if openIdx < 0 || openIdx >= len(src) || src[openIdx] != '(' {
		return -1, fmt.Errorf("backend: expected '(' at %d", openIdx)
	}
	depth := 0
	inString := false
	inLineComment := false
	inBlockComment := false
	for i := openIdx; i < len(src); i++ {
		ch := src[i]
		next := byte(0)
		if i+1 < len(src) {
			next = src[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if ch == '\\' {
				i++
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && next == '/' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("backend: unmatched '(' in fwrite")
}
