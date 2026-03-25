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
