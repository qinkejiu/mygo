package backend

import (
	"strings"
	"testing"

	"mygo/internal/ir"
)

func TestRewriteFwriteCallsUsesWriteForStdout(t *testing.T) {
	src := `module main;
initial begin
  $fwrite(32'h80000001, "hello");
  $fwrite(32'h80000002, "side");
end
endmodule
`

	got, _, err := rewriteFwriteCalls(src, nil)
	if err != nil {
		t.Fatalf("rewriteFwriteCalls failed: %v", err)
	}
	if !strings.Contains(got, `$write("hello")`) {
		t.Fatalf("expected stdout fwrite to become $write, got:\n%s", got)
	}
	if strings.Contains(got, `$display("hello")`) {
		t.Fatalf("expected stdout fwrite not to become $display, got:\n%s", got)
	}
	if !strings.Contains(got, `$fwrite(32'h80000002, "side")`) {
		t.Fatalf("expected non-stdout fwrite to remain fwrite, got:\n%s", got)
	}
}

func TestRewriteFwriteCallsFormatsBoolAsTrueFalse(t *testing.T) {
	src := `module main;
initial begin
  $fwrite(32'h80000001, "finished is %0s\n", done);
end
endmodule
`
	prints := []printInfo{
		{
			operands: []printOperandInfo{
				{width: 1, verb: ir.PrintVerbBool},
			},
		},
	}

	got, _, err := rewriteFwriteCalls(src, prints)
	if err != nil {
		t.Fatalf("rewriteFwriteCalls failed: %v", err)
	}
	if !strings.Contains(got, `$write("finished is %0s\n", ((done) ? "true" : "false"))`) {
		t.Fatalf("expected bool operand rewrite, got:\n%s", got)
	}
}

func TestRewriteSignedDeclsAndAssignsScopesNamesByModule(t *testing.T) {
	src := `module alpha;
  wire [31:0] v0 = 32'h1;
endmodule
module beta;
  wire [31:0] v0 = 32'h2;
endmodule
`

	got := rewriteSignedDeclsAndAssigns(src, signedNamesByModule{
		"alpha": map[string]struct{}{"v0": {}},
	})

	if !strings.Contains(got, "wire signed [31:0] v0 = 32'sh1;") {
		t.Fatalf("expected alpha.v0 to become signed, got:\n%s", got)
	}
	if strings.Contains(got, "module beta;\n  wire signed [31:0] v0 = 32'sh2;") {
		t.Fatalf("expected beta.v0 to remain unsigned, got:\n%s", got)
	}
	if !strings.Contains(got, "module beta;\n  wire [31:0] v0 = 32'h2;") {
		t.Fatalf("expected beta.v0 declaration to stay unchanged, got:\n%s", got)
	}
}

func TestRewriteFwriteCallsTracksSignedNamesByModule(t *testing.T) {
	src := `module alpha;
  wire [31:0] v0;
  initial begin
    $fwrite(32'h80000001, "%0d\n", v0);
  end
endmodule
module beta;
  wire [31:0] v0 = 32'h2;
endmodule
`
	prints := []printInfo{
		{
			operands: []printOperandInfo{
				{signed: true, width: 32, verb: ir.PrintVerbDec},
			},
		},
	}

	rewritten, signedNames, err := rewriteFwriteCalls(src, prints)
	if err != nil {
		t.Fatalf("rewriteFwriteCalls failed: %v", err)
	}
	got := rewriteSignedDeclsAndAssigns(rewritten, signedNames)

	if !strings.Contains(got, "module alpha;\n  wire signed [31:0] v0;") {
		t.Fatalf("expected alpha.v0 to become signed from print rewrite, got:\n%s", got)
	}
	if strings.Contains(got, "module beta;\n  wire signed [31:0] v0 = 32'sh2;") {
		t.Fatalf("expected beta.v0 to remain unsigned, got:\n%s", got)
	}
	if !strings.Contains(got, "module beta;\n  wire [31:0] v0 = 32'h2;") {
		t.Fatalf("expected beta.v0 declaration to stay unchanged, got:\n%s", got)
	}
}
