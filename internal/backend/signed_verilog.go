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
	verb   ir.PrintVerb
}

type printInfo struct {
	operands []printOperandInfo
}

var (
	verilogLiteralRe = regexp.MustCompile(`\b(\d+)'([bBdDhH])([0-9a-fA-F_xXzZ?]+)\b`)
	declRe           = regexp.MustCompile(`^(\s*)(wire|reg|inout|input|output)\s+(signed\s+)?(\[[^\]]+\]\s+)?([A-Za-z_][A-Za-z0-9_$]*)\b(.*)$`)
	assignRe         = regexp.MustCompile(`^(\s*)assign\s+([A-Za-z_][A-Za-z0-9_$]*)\s*=\s*(.*?);\s*$`)
	moduleStartRe    = regexp.MustCompile(`^\s*module\s+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	endmoduleRe      = regexp.MustCompile(`^\s*endmodule\b`)
)

type signedNamesByModule map[string]map[string]struct{}

type verilogModuleSpan struct {
	name  string
	start int
	end   int
}

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
	mergeSignedNames(signedNames, printNames)
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
							verb:   seg.Verb,
						})
					}
					prints = append(prints, info)
				}
			}
		}
	}
	return prints
}

func collectSignedNames(design *ir.Design) signedNamesByModule {
	signed := make(signedNamesByModule)
	if design == nil {
		return signed
	}
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		moduleName := sanitize(module.Name)
		for _, port := range module.Ports {
			if port.Type == nil || !port.Type.Signed || port.Type.Width <= 1 {
				continue
			}
			signed.add(moduleName, sanitize(port.Name))
		}
		for _, sig := range module.Signals {
			if sig == nil || sig.Type == nil || !sig.Type.Signed || sig.Type.Width <= 1 {
				continue
			}
			signed.add(moduleName, sanitize(sig.Name))
		}
		for _, ch := range module.Channels {
			if ch == nil || ch.Type == nil || !ch.Type.Signed || ch.Type.Width <= 1 {
				continue
			}
			name := sanitize(ch.Name)
			signed.add(moduleName, "chan_"+name+"_wdata")
			signed.add(moduleName, "chan_"+name+"_rdata")
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

func rewriteFwriteCalls(src string, prints []printInfo) (string, signedNamesByModule, error) {
	signedNames := make(signedNamesByModule)
	moduleSpans := parseVerilogModuleSpans(src)
	var out strings.Builder
	idx := 0
	i := 0
	moduleIdx := 0
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
		// Check if this is an $fwrite to stdout (32'h80000001).
		// Preserve Go fmt semantics by lowering stdout writes to $write rather than
		// $display, because newlines are already encoded in the format string when
		// the source used fmt.Println/fmt.Printf with \n.
		isStdout := false
		if len(args) > 0 {
			firstArg := strings.TrimSpace(args[0])
			if firstArg == "32'h80000001" || firstArg == "0x80000001" {
				isStdout = true
				// Remove the first argument (file descriptor)
				args = args[1:]
				seps = seps[1:]
			}
		}

		if idx < len(prints) {
			currentModule := moduleNameForOffset(moduleSpans, start, &moduleIdx)
			var localSigned map[string]struct{}
			args, localSigned = rewriteFwriteArgs(args, prints[idx], nil)
			for name := range localSigned {
				signedNames.add(currentModule, name)
			}
			idx++
		}

		// Use $write for stdout, $fwrite for other file descriptors.
		if isStdout {
			out.WriteString("$write(")
		} else {
			out.WriteString("$fwrite(")
		}
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
	operandStart := 2
	if strings.HasPrefix(strings.TrimSpace(args[0]), "\"") {
		operandStart = 1
	}
	for i := operandStart; i < len(args); i++ {
		opIdx := i - operandStart
		if opIdx >= len(info.operands) {
			break
		}
		operand := info.operands[opIdx]
		if operand.verb == ir.PrintVerbBool {
			raw := args[i]
			leading, core, trailing := trimArg(raw)
			if core == "" {
				continue
			}
			args[i] = leading + "((" + core + ") ? \"true\" : \"false\")" + trailing
			continue
		}
		if operand.verb == ir.PrintVerbFloat {
			raw := args[i]
			leading, core, trailing := trimArg(raw)
			if core == "" {
				continue
			}
			if operand.width == 64 {
				core = "$bitstoreal(" + core + ")"
			}
			args[i] = leading + core + trailing
			continue
		}
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

func rewriteSignedDeclsAndAssigns(src string, signedNames signedNamesByModule) string {
	if len(signedNames) == 0 {
		return src
	}
	lines := strings.Split(src, "\n")
	currentModule := ""
	for i, line := range lines {
		if matches := moduleStartRe.FindStringSubmatch(line); matches != nil {
			currentModule = matches[1]
			lines[i] = line
			continue
		}
		if endmoduleRe.MatchString(line) {
			currentModule = ""
			lines[i] = line
			continue
		}
		if currentModule == "" {
			lines[i] = line
			continue
		}
		if matches := declRe.FindStringSubmatch(line); matches != nil {
			name := matches[5]
			if signedNames.has(currentModule, name) && matches[3] == "" && matches[4] != "" {
				line = matches[1] + matches[2] + " signed " + matches[4] + name + matches[6]
			}
			if signedNames.has(currentModule, name) {
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
			if signedNames.has(currentModule, name) {
				expr := rewriteSignedLiterals(matches[3])
				line = matches[1] + "assign " + name + " = " + expr + ";"
				lines[i] = line
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (s signedNamesByModule) add(moduleName, name string) {
	if moduleName == "" || name == "" {
		return
	}
	if s[moduleName] == nil {
		s[moduleName] = make(map[string]struct{})
	}
	s[moduleName][name] = struct{}{}
}

func (s signedNamesByModule) has(moduleName, name string) bool {
	if moduleName == "" || name == "" {
		return false
	}
	names := s[moduleName]
	if len(names) == 0 {
		return false
	}
	_, ok := names[name]
	return ok
}

func mergeSignedNames(dst, src signedNamesByModule) {
	for moduleName, names := range src {
		for name := range names {
			dst.add(moduleName, name)
		}
	}
}

func parseVerilogModuleSpans(src string) []verilogModuleSpan {
	lines := strings.SplitAfter(src, "\n")
	if len(lines) == 0 {
		return nil
	}
	spans := make([]verilogModuleSpan, 0, 4)
	currentName := ""
	currentStart := 0
	offset := 0
	for _, line := range lines {
		if currentName == "" {
			if matches := moduleStartRe.FindStringSubmatch(line); matches != nil {
				currentName = matches[1]
				currentStart = offset
			}
		}
		offset += len(line)
		if currentName != "" && endmoduleRe.MatchString(line) {
			spans = append(spans, verilogModuleSpan{
				name:  currentName,
				start: currentStart,
				end:   offset,
			})
			currentName = ""
		}
	}
	if currentName != "" {
		spans = append(spans, verilogModuleSpan{
			name:  currentName,
			start: currentStart,
			end:   len(src),
		})
	}
	return spans
}

func moduleNameForOffset(spans []verilogModuleSpan, offset int, idx *int) string {
	if idx == nil {
		i := 0
		idx = &i
	}
	for *idx < len(spans) && offset >= spans[*idx].end {
		*idx = *idx + 1
	}
	if *idx < len(spans) {
		span := spans[*idx]
		if offset >= span.start && offset < span.end {
			return span.name
		}
	}
	return ""
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
