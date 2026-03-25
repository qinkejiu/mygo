package backend

import (
	"strings"
	"testing"

	"mygo/internal/ir"
)

func TestRewriteFwriteCallsCastsFloatOperands(t *testing.T) {
	src := `module main;
initial begin
  $fwrite(32'h80000001, "%f\n", value_bits);
end
endmodule
`
	prints := []printInfo{
		{
			operands: []printOperandInfo{
				{width: 64, verb: ir.PrintVerbFloat},
			},
		},
	}
	got, _, err := rewriteFwriteCalls(src, prints)
	if err != nil {
		t.Fatalf("rewriteFwriteCalls failed: %v", err)
	}
	if !strings.Contains(got, `$write("%f\n", $bitstoreal(value_bits))`) {
		t.Fatalf("expected float operand cast via $bitstoreal, got:\n%s", got)
	}
}

func TestRewriteFwriteCallsPreservesMinimalHexVerb(t *testing.T) {
	src := `module main;
initial begin
  $fwrite(32'h80000001, "value=%0x\n", value_bits);
end
endmodule
`
	prints := []printInfo{
		{
			operands: []printOperandInfo{
				{width: 32, verb: ir.PrintVerbHex},
			},
		},
	}
	got, _, err := rewriteFwriteCalls(src, prints)
	if err != nil {
		t.Fatalf("rewriteFwriteCalls failed: %v", err)
	}
	if !strings.Contains(got, `$write("value=%0x\n", value_bits)`) {
		t.Fatalf("expected minimal-width hex verb, got:\n%s", got)
	}
}
