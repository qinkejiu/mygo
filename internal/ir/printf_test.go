package ir

import "testing"

const printfZeroPaddedHexProgram = `
package main

import "fmt"

func main() {
	var v uint64
	v = 0x2a
	fmt.Printf("value=%016x\n", v)
}
`

const printfWidthOnlyHexProgram = `
package main

import "fmt"

func main() {
	var v uint64
	v = 0x2a
	fmt.Printf("value=%16x\n", v)
}
`

const printfBoolProgram = `
package main

import "fmt"

func main() {
	done := true
	fmt.Printf("done=%t verbose=%v\n", done, done)
}
`

func TestPrintfParsesZeroPaddedHexWidth(t *testing.T) {
	design := buildDesignFromSource(t, printfZeroPaddedHexProgram)
	seg := firstValuePrintSegment(t, design)
	if seg.Verb != PrintVerbHex {
		t.Fatalf("verb = %v, want %v", seg.Verb, PrintVerbHex)
	}
	if seg.Width != 16 {
		t.Fatalf("width = %d, want 16", seg.Width)
	}
	if !seg.ZeroPad {
		t.Fatalf("zeroPad = false, want true")
	}
}

func TestPrintfParsesWidthWithoutZeroPad(t *testing.T) {
	design := buildDesignFromSource(t, printfWidthOnlyHexProgram)
	seg := firstValuePrintSegment(t, design)
	if seg.Verb != PrintVerbHex {
		t.Fatalf("verb = %v, want %v", seg.Verb, PrintVerbHex)
	}
	if seg.Width != 16 {
		t.Fatalf("width = %d, want 16", seg.Width)
	}
	if seg.ZeroPad {
		t.Fatalf("zeroPad = true, want false")
	}
}

func TestPrintfParsesBoolVerbs(t *testing.T) {
	design := buildDesignFromSource(t, printfBoolProgram)
	segments := valuePrintSegments(t, design)
	if len(segments) != 2 {
		t.Fatalf("got %d value print segments, want 2", len(segments))
	}
	for i, seg := range segments {
		if seg.Verb != PrintVerbBool {
			t.Fatalf("segment %d verb = %v, want %v", i, seg.Verb, PrintVerbBool)
		}
	}
}

const printNilProgram = `
package main

import "fmt"

func main() {
	fmt.Print(nil)
}
`

const remainderProgram = `
package main

import "fmt"

func main() {
	var a int
	var b int
	a = 7
	b = 3
	fmt.Printf("%d\n", a%b)
}
`

func TestPrintNilDoesNotPanic(t *testing.T) {
	design := buildDesignFromSource(t, printNilProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("missing design after fmt.Print(nil)")
	}
}

func TestRemainderBuildsBinOp(t *testing.T) {
	design := buildDesignFromSource(t, remainderProgram)
	if !designContainsBinOp(design, Rem) {
		t.Fatalf("expected design to contain remainder binop")
	}
}

func firstValuePrintSegment(t *testing.T, design *Design) PrintSegment {
	t.Helper()
	segments := valuePrintSegments(t, design)
	if len(segments) == 0 {
		t.Fatalf("no print value segment found")
	}
	return segments[0]
}

func valuePrintSegments(t *testing.T, design *Design) []PrintSegment {
	t.Helper()
	if design == nil || design.TopLevel == nil {
		t.Fatalf("missing top-level design")
	}
	var out []PrintSegment
	for _, proc := range design.TopLevel.Processes {
		if proc == nil {
			continue
		}
		for _, block := range proc.Blocks {
			if block == nil {
				continue
			}
			for _, op := range block.Ops {
				printOp, ok := op.(*PrintOperation)
				if !ok || printOp == nil {
					continue
				}
				for _, seg := range printOp.Segments {
					if seg.Value != nil {
						out = append(out, seg)
					}
				}
			}
		}
	}
	return out
}

func designContainsBinOp(design *Design, want BinOp) bool {
	if design == nil || design.TopLevel == nil {
		return false
	}
	for _, proc := range design.TopLevel.Processes {
		if proc == nil {
			continue
		}
		for _, block := range proc.Blocks {
			if block == nil {
				continue
			}
			for _, op := range block.Ops {
				bin, ok := op.(*BinOperation)
				if ok && bin != nil && bin.Op == want {
					return true
				}
			}
		}
	}
	return false
}
