package backend

import (
	"fmt"
	"math/bits"
	"strings"
)

// GenerateFIFOVerilog renders a synthesizable single-clock FIFO module.
// The generated module uses active-low reset (rst_n), blocks writes on full,
// and blocks reads on empty.
func GenerateFIFOVerilog(moduleName string, dataWidth int, depth int, isAsyncReset bool, almostFullLevel int) string {
	name := sanitize(moduleName)
	if name == "" {
		name = "mygo_fifo"
	}
	if dataWidth <= 0 {
		dataWidth = 1
	}
	if depth <= 0 {
		depth = 1
	}

	addrWidth := fifoAddrWidth(depth)
	countWidth := fifoCountWidth(depth)
	almostFullLevel = clampAlmostFullLevel(depth, almostFullLevel)
	largeFifo := depth > 64

	dataRange := fifoDataRange(dataWidth)
	resetHeader := "always @(posedge clk)"
	if isAsyncReset {
		resetHeader = "always @(posedge clk or negedge rst_n)"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "module %s (\n", name)
	fmt.Fprintf(&b, "  input  wire                   clk,\n")
	fmt.Fprintf(&b, "  input  wire                   rst_n,\n")
	fmt.Fprintf(&b, "  input  wire                   wr_en,\n")
	fmt.Fprintf(&b, "  input  wire %s          wr_data,\n", dataRange)
	fmt.Fprintf(&b, "  output wire                   full,\n")
	fmt.Fprintf(&b, "  output wire                   almost_full,\n")
	fmt.Fprintf(&b, "  input  wire                   rd_en,\n")
	fmt.Fprintf(&b, "  output wire %s          rd_data,\n", dataRange)
	fmt.Fprintf(&b, "  output wire                   empty,\n")
	fmt.Fprintf(&b, "  output wire                   almost_empty\n")
	fmt.Fprintf(&b, ");\n\n")

	fmt.Fprintf(&b, "  localparam integer DATA_WIDTH = %d;\n", dataWidth)
	fmt.Fprintf(&b, "  localparam integer DEPTH = %d;\n", depth)
	fmt.Fprintf(&b, "  localparam integer ADDR_WIDTH = %d;\n", addrWidth)
	fmt.Fprintf(&b, "  localparam integer COUNT_WIDTH = %d;\n", countWidth)
	fmt.Fprintf(&b, "  localparam integer ALMOST_FULL_LEVEL = %d;\n\n", almostFullLevel)

	fmt.Fprintf(&b, "  localparam [ADDR_WIDTH-1:0] LAST_PTR = ADDR_WIDTH'(DEPTH - 1);\n")
	fmt.Fprintf(&b, "  localparam [COUNT_WIDTH-1:0] DEPTH_COUNT = COUNT_WIDTH'(DEPTH);\n")
	fmt.Fprintf(&b, "  localparam [COUNT_WIDTH-1:0] ALMOST_FULL_COUNT = COUNT_WIDTH'(ALMOST_FULL_LEVEL);\n")
	fmt.Fprintf(&b, "  localparam [COUNT_WIDTH-1:0] ALMOST_EMPTY_COUNT = COUNT_WIDTH'(1);\n\n")

	if largeFifo {
		fmt.Fprintf(&b, "  // RAM-oriented style for deeper FIFOs.\n")
	} else {
		fmt.Fprintf(&b, "  // Register-based circular buffer for shallow FIFOs.\n")
	}
	fmt.Fprintf(&b, "  reg [DATA_WIDTH-1:0] mem [0:DEPTH-1];\n")
	fmt.Fprintf(&b, "  reg [ADDR_WIDTH-1:0] wr_ptr;\n")
	fmt.Fprintf(&b, "  reg [ADDR_WIDTH-1:0] rd_ptr;\n")
	fmt.Fprintf(&b, "  reg [COUNT_WIDTH-1:0] count;\n")
	if largeFifo {
		fmt.Fprintf(&b, "  reg [DATA_WIDTH-1:0] rd_data_reg;\n")
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "  wire wr_fire = wr_en && !full;\n")
	fmt.Fprintf(&b, "  wire rd_fire = rd_en && !empty;\n")
	fmt.Fprintf(&b, "  wire [ADDR_WIDTH-1:0] wr_ptr_next = (wr_ptr == LAST_PTR) ? {ADDR_WIDTH{1'b0}} : (wr_ptr + 1'b1);\n")
	fmt.Fprintf(&b, "  wire [ADDR_WIDTH-1:0] rd_ptr_next = (rd_ptr == LAST_PTR) ? {ADDR_WIDTH{1'b0}} : (rd_ptr + 1'b1);\n\n")

	fmt.Fprintf(&b, "  assign full = (count == DEPTH_COUNT);\n")
	fmt.Fprintf(&b, "  assign empty = (count == {COUNT_WIDTH{1'b0}});\n")
	fmt.Fprintf(&b, "  assign almost_full = (count >= ALMOST_FULL_COUNT);\n")
	if depth <= 1 {
		fmt.Fprintf(&b, "  assign almost_empty = empty;\n")
	} else {
		fmt.Fprintf(&b, "  assign almost_empty = (count <= ALMOST_EMPTY_COUNT);\n")
	}
	if largeFifo {
		fmt.Fprintf(&b, "  assign rd_data = empty ? {DATA_WIDTH{1'b0}} : rd_data_reg;\n")
	} else {
		fmt.Fprintf(&b, "  assign rd_data = empty ? {DATA_WIDTH{1'b0}} : mem[rd_ptr];\n")
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "  %s begin\n", resetHeader)
	fmt.Fprintf(&b, "    if (!rst_n) begin\n")
	fmt.Fprintf(&b, "      wr_ptr <= {ADDR_WIDTH{1'b0}};\n")
	fmt.Fprintf(&b, "      rd_ptr <= {ADDR_WIDTH{1'b0}};\n")
	fmt.Fprintf(&b, "      count <= {COUNT_WIDTH{1'b0}};\n")
	if largeFifo {
		fmt.Fprintf(&b, "      rd_data_reg <= {DATA_WIDTH{1'b0}};\n")
	}
	fmt.Fprintf(&b, "    end else begin\n")
	fmt.Fprintf(&b, "      if (wr_fire) begin\n")
	fmt.Fprintf(&b, "        mem[wr_ptr] <= wr_data;\n")
	fmt.Fprintf(&b, "        wr_ptr <= wr_ptr_next;\n")
	fmt.Fprintf(&b, "      end\n")
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "      if (rd_fire) begin\n")
	fmt.Fprintf(&b, "        rd_ptr <= rd_ptr_next;\n")
	fmt.Fprintf(&b, "      end\n")
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "      case ({wr_fire, rd_fire})\n")
	fmt.Fprintf(&b, "        2'b10: count <= count + 1'b1;\n")
	fmt.Fprintf(&b, "        2'b01: count <= count - 1'b1;\n")
	fmt.Fprintf(&b, "        default: count <= count;\n")
	fmt.Fprintf(&b, "      endcase\n")
	if largeFifo {
		fmt.Fprintf(&b, "\n")
		fmt.Fprintf(&b, "      if (empty && wr_fire) begin\n")
		fmt.Fprintf(&b, "        rd_data_reg <= wr_data;\n")
		fmt.Fprintf(&b, "      end else if (rd_fire) begin\n")
		fmt.Fprintf(&b, "        if (count > 1) begin\n")
		fmt.Fprintf(&b, "          rd_data_reg <= mem[rd_ptr_next];\n")
		fmt.Fprintf(&b, "        end else if (wr_fire) begin\n")
		fmt.Fprintf(&b, "          rd_data_reg <= wr_data;\n")
		fmt.Fprintf(&b, "        end else begin\n")
		fmt.Fprintf(&b, "          rd_data_reg <= {DATA_WIDTH{1'b0}};\n")
		fmt.Fprintf(&b, "        end\n")
		fmt.Fprintf(&b, "      end else if (!empty) begin\n")
		fmt.Fprintf(&b, "        rd_data_reg <= mem[rd_ptr];\n")
		fmt.Fprintf(&b, "      end\n")
	}
	fmt.Fprintf(&b, "    end\n")
	fmt.Fprintf(&b, "  end\n")
	fmt.Fprintf(&b, "endmodule")

	return b.String()
}

func fifoAddrWidth(depth int) int {
	if depth <= 1 {
		return 1
	}
	return bits.Len(uint(depth - 1))
}

func fifoCountWidth(depth int) int {
	if depth <= 1 {
		return 1
	}
	return bits.Len(uint(depth))
}

func clampAlmostFullLevel(depth int, level int) int {
	if depth <= 1 {
		return 1
	}
	if level <= 0 {
		level = depth - 1
	}
	if level > depth {
		level = depth
	}
	return level
}

func fifoDataRange(width int) string {
	if width <= 1 {
		return ""
	}
	return fmt.Sprintf("[%d:0]", width-1)
}
