package mlir

import (
	"testing"

	"mygo/internal/ir"
)

func TestPrintVerbSpecifier(t *testing.T) {
	tests := []struct {
		name string
		seg  ir.PrintSegment
		want string
	}{
		{
			name: "decimal default",
			seg:  ir.PrintSegment{Verb: ir.PrintVerbDec},
			want: "%d",
		},
		{
			name: "hex zero padded width",
			seg: ir.PrintSegment{
				Verb:    ir.PrintVerbHex,
				Width:   16,
				ZeroPad: true,
			},
			want: "%016x",
		},
		{
			name: "hex width no zero pad",
			seg: ir.PrintSegment{
				Verb:  ir.PrintVerbHex,
				Width: 8,
			},
			want: "%8x",
		},
		{
			name: "zero pad ignored without width",
			seg: ir.PrintSegment{
				Verb:    ir.PrintVerbBin,
				ZeroPad: true,
			},
			want: "%b",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := printVerbSpecifier(tc.seg)
			if got != tc.want {
				t.Fatalf("printVerbSpecifier() = %q, want %q", got, tc.want)
			}
		})
	}
}
