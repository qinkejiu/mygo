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

func firstValuePrintSegment(t *testing.T, design *Design) PrintSegment {
	t.Helper()
	if design == nil || design.TopLevel == nil {
		t.Fatalf("missing top-level design")
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
				printOp, ok := op.(*PrintOperation)
				if !ok || printOp == nil {
					continue
				}
				for _, seg := range printOp.Segments {
					if seg.Value != nil {
						return seg
					}
				}
			}
		}
	}
	t.Fatalf("no print value segment found")
	return PrintSegment{}
}
