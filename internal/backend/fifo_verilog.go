package backend

import (
	"fmt"
	"strings"

	"mygo/internal/ir"
)

const reusableFIFOName = "mygo_fifo"

type fifoSpec struct {
	ModuleName            string
	DataWidth             int
	Depth                 int
	AddrWidth             int
	CountWidth            int
	AsyncReset            bool
	AlmostFullLevel       int
	AlmostEmptyLevel      int
	LastPtrValue          int
	DepthCountValue       int
	AlmostFullCountValue  int
	AlmostEmptyCountValue int
	UseRegisteredRead     bool
	AlmostEmptyUsesEmpty  bool
}

// GenerateFIFOVerilog renders a concrete FIFO module for the supplied parameters.
// All sizing and policy decisions are made before rendering.
func GenerateFIFOVerilog(moduleName string, dataWidth int, depth int, isAsyncReset bool, almostFullLevel int) string {
	spec := buildConcreteFIFOSpec(moduleName, dataWidth, depth, isAsyncReset, almostFullLevel)
	return renderConcreteFIFOVerilog(spec)
}

// GenerateFIFOVerilogFromDecl renders Verilog for a lowered structural FIFO declaration.
func GenerateFIFOVerilogFromDecl(decl *ir.FIFODecl) string {
	return renderConcreteFIFOVerilog(buildSpecFromDecl(decl))
}

// GenerateReusableParametricFIFOVerilog renders the single reusable FIFO module.
// All derived values are passed in as parameters from Go-side lowering.
func GenerateReusableParametricFIFOVerilog(moduleName string) string {
	name := sanitize(moduleName)
	if name == "" {
		name = reusableFIFOName
	}

	var b strings.Builder
	fmt.Fprintf(&b, "module %s #(\n", name)
	fmt.Fprintf(&b, "  parameter integer DATA_WIDTH = 32,\n")
	fmt.Fprintf(&b, "  parameter integer DEPTH = 1,\n")
	fmt.Fprintf(&b, "  parameter integer ADDR_WIDTH = 1,\n")
	fmt.Fprintf(&b, "  parameter integer COUNT_WIDTH = 1,\n")
	fmt.Fprintf(&b, "  parameter integer LAST_PTR_VALUE = 0,\n")
	fmt.Fprintf(&b, "  parameter integer DEPTH_COUNT_VALUE = 1,\n")
	fmt.Fprintf(&b, "  parameter integer ALMOST_FULL_LEVEL = 1,\n")
	fmt.Fprintf(&b, "  parameter integer ALMOST_EMPTY_LEVEL = 0,\n")
	fmt.Fprintf(&b, "  parameter integer ALMOST_FULL_COUNT_VALUE = 1,\n")
	fmt.Fprintf(&b, "  parameter integer ALMOST_EMPTY_COUNT_VALUE = 0,\n")
	fmt.Fprintf(&b, "  parameter bit USE_REGISTERED_READ = 1'b0,\n")
	fmt.Fprintf(&b, "  parameter bit ALMOST_EMPTY_USES_EMPTY = 1'b1,\n")
	fmt.Fprintf(&b, "  parameter bit ASYNC_RESET = 1'b0\n")
	fmt.Fprintf(&b, ") (\n")
	fmt.Fprintf(&b, "  input  wire                   clk,\n")
	fmt.Fprintf(&b, "  input  wire                   rst_n,\n")
	fmt.Fprintf(&b, "  input  wire                   wr_en,\n")
	fmt.Fprintf(&b, "  input  wire [DATA_WIDTH-1:0]  wr_data,\n")
	fmt.Fprintf(&b, "  output wire                   full,\n")
	fmt.Fprintf(&b, "  output wire                   almost_full,\n")
	fmt.Fprintf(&b, "  input  wire                   rd_en,\n")
	fmt.Fprintf(&b, "  output wire [DATA_WIDTH-1:0]  rd_data,\n")
	fmt.Fprintf(&b, "  output wire                   empty,\n")
	fmt.Fprintf(&b, "  output wire                   almost_empty\n")
	fmt.Fprintf(&b, ");\n\n")

	fmt.Fprintf(&b, "  localparam [ADDR_WIDTH-1:0] LAST_PTR = ADDR_WIDTH'(LAST_PTR_VALUE);\n")
	fmt.Fprintf(&b, "  localparam [COUNT_WIDTH-1:0] DEPTH_COUNT = COUNT_WIDTH'(DEPTH_COUNT_VALUE);\n")
	fmt.Fprintf(&b, "  localparam [COUNT_WIDTH-1:0] ALMOST_FULL_COUNT = COUNT_WIDTH'(ALMOST_FULL_COUNT_VALUE);\n")
	fmt.Fprintf(&b, "  localparam [COUNT_WIDTH-1:0] ALMOST_EMPTY_COUNT = COUNT_WIDTH'(ALMOST_EMPTY_COUNT_VALUE);\n\n")

	fmt.Fprintf(&b, "  reg [DATA_WIDTH-1:0] mem [0:DEPTH-1];\n")
	fmt.Fprintf(&b, "  reg [ADDR_WIDTH-1:0] wr_ptr;\n")
	fmt.Fprintf(&b, "  reg [ADDR_WIDTH-1:0] rd_ptr;\n")
	fmt.Fprintf(&b, "  reg [COUNT_WIDTH-1:0] count;\n\n")

	fmt.Fprintf(&b, "  wire wr_fire = wr_en && !full;\n")
	fmt.Fprintf(&b, "  wire rd_fire = rd_en && !empty;\n")
	fmt.Fprintf(&b, "  wire [ADDR_WIDTH-1:0] wr_ptr_next = (wr_ptr == LAST_PTR) ? {ADDR_WIDTH{1'b0}} : (wr_ptr + 1'b1);\n")
	fmt.Fprintf(&b, "  wire [ADDR_WIDTH-1:0] rd_ptr_next = (rd_ptr == LAST_PTR) ? {ADDR_WIDTH{1'b0}} : (rd_ptr + 1'b1);\n\n")

	fmt.Fprintf(&b, "  assign full = (count == DEPTH_COUNT);\n")
	fmt.Fprintf(&b, "  assign empty = (count == {COUNT_WIDTH{1'b0}});\n")
	fmt.Fprintf(&b, "  assign almost_full = (count >= ALMOST_FULL_COUNT);\n")
	fmt.Fprintf(&b, "  assign almost_empty = (ALMOST_EMPTY_USES_EMPTY != 1'b0) ? empty : (count <= ALMOST_EMPTY_COUNT);\n\n")

	fmt.Fprintf(&b, "  generate\n")
	fmt.Fprintf(&b, "    if (USE_REGISTERED_READ != 1'b0) begin : gen_registered_read\n")
	fmt.Fprintf(&b, "      reg [DATA_WIDTH-1:0] rd_data_reg;\n")
	fmt.Fprintf(&b, "      assign rd_data = empty ? {DATA_WIDTH{1'b0}} : rd_data_reg;\n\n")
	renderReusableSequentialBlock(&b, true)
	fmt.Fprintf(&b, "    end else begin : gen_direct_read\n")
	fmt.Fprintf(&b, "      assign rd_data = empty ? {DATA_WIDTH{1'b0}} : mem[rd_ptr];\n\n")
	renderReusableSequentialBlock(&b, false)
	fmt.Fprintf(&b, "    end\n")
	fmt.Fprintf(&b, "  endgenerate\n")
	fmt.Fprintf(&b, "endmodule")
	return b.String()
}

func renderReusableSequentialBlock(b *strings.Builder, registeredRead bool) {
	fmt.Fprintf(b, "      if (ASYNC_RESET != 1'b0) begin : gen_async_reset\n")
	fmt.Fprintf(b, "        always @(posedge clk or negedge rst_n) begin\n")
	fmt.Fprintf(b, "          if (!rst_n) begin\n")
	renderReusableResetBody(b, registeredRead)
	fmt.Fprintf(b, "          end else begin\n")
	renderReusableActiveBody(b, registeredRead)
	fmt.Fprintf(b, "          end\n")
	fmt.Fprintf(b, "        end\n")
	fmt.Fprintf(b, "      end else begin : gen_sync_reset\n")
	fmt.Fprintf(b, "        always @(posedge clk) begin\n")
	fmt.Fprintf(b, "          if (!rst_n) begin\n")
	renderReusableResetBody(b, registeredRead)
	fmt.Fprintf(b, "          end else begin\n")
	renderReusableActiveBody(b, registeredRead)
	fmt.Fprintf(b, "          end\n")
	fmt.Fprintf(b, "        end\n")
	fmt.Fprintf(b, "      end\n")
}

func renderReusableResetBody(b *strings.Builder, registeredRead bool) {
	fmt.Fprintf(b, "            wr_ptr <= {ADDR_WIDTH{1'b0}};\n")
	fmt.Fprintf(b, "            rd_ptr <= {ADDR_WIDTH{1'b0}};\n")
	fmt.Fprintf(b, "            count <= {COUNT_WIDTH{1'b0}};\n")
	if registeredRead {
		fmt.Fprintf(b, "            rd_data_reg <= {DATA_WIDTH{1'b0}};\n")
	}
}

func renderReusableActiveBody(b *strings.Builder, registeredRead bool) {
	fmt.Fprintf(b, "            if (wr_fire) begin\n")
	fmt.Fprintf(b, "              mem[wr_ptr] <= wr_data;\n")
	fmt.Fprintf(b, "              wr_ptr <= wr_ptr_next;\n")
	fmt.Fprintf(b, "            end\n")
	fmt.Fprintf(b, "            if (rd_fire) begin\n")
	fmt.Fprintf(b, "              rd_ptr <= rd_ptr_next;\n")
	fmt.Fprintf(b, "            end\n")
	fmt.Fprintf(b, "            case ({wr_fire, rd_fire})\n")
	fmt.Fprintf(b, "              2'b10: count <= count + 1'b1;\n")
	fmt.Fprintf(b, "              2'b01: count <= count - 1'b1;\n")
	fmt.Fprintf(b, "              default: count <= count;\n")
	fmt.Fprintf(b, "            endcase\n")
	if registeredRead {
		fmt.Fprintf(b, "            if (empty && wr_fire) begin\n")
		fmt.Fprintf(b, "              rd_data_reg <= wr_data;\n")
		fmt.Fprintf(b, "            end else if (rd_fire) begin\n")
		fmt.Fprintf(b, "              if (count > 1) begin\n")
		fmt.Fprintf(b, "                rd_data_reg <= mem[rd_ptr_next];\n")
		fmt.Fprintf(b, "              end else if (wr_fire) begin\n")
		fmt.Fprintf(b, "                rd_data_reg <= wr_data;\n")
		fmt.Fprintf(b, "              end else begin\n")
		fmt.Fprintf(b, "                rd_data_reg <= {DATA_WIDTH{1'b0}};\n")
		fmt.Fprintf(b, "              end\n")
		fmt.Fprintf(b, "            end else if (!empty) begin\n")
		fmt.Fprintf(b, "              rd_data_reg <= mem[rd_ptr];\n")
		fmt.Fprintf(b, "            end\n")
	}
}

func buildConcreteFIFOSpec(moduleName string, dataWidth int, depth int, asyncReset bool, almostFullLevel int) fifoSpec {
	if dataWidth <= 0 {
		dataWidth = 1
	}
	if depth <= 0 {
		depth = 1
	}
	almostFullLevel = clampAlmostFullLevel(depth, almostFullLevel)
	almostEmptyLevel := defaultAlmostEmptyLevel(depth)
	return fifoSpec{
		ModuleName:            sanitizedModuleName(moduleName),
		DataWidth:             dataWidth,
		Depth:                 depth,
		AddrWidth:             fifoAddrWidth(depth),
		CountWidth:            fifoCountWidth(depth),
		AsyncReset:            asyncReset,
		AlmostFullLevel:       almostFullLevel,
		AlmostEmptyLevel:      almostEmptyLevel,
		LastPtrValue:          depth - 1,
		DepthCountValue:       depth,
		AlmostFullCountValue:  almostFullLevel,
		AlmostEmptyCountValue: almostEmptyLevel,
		UseRegisteredRead:     depth > 64,
		AlmostEmptyUsesEmpty:  depth <= 1,
	}
}

func buildSpecFromDecl(decl *ir.FIFODecl) fifoSpec {
	if decl == nil {
		return buildConcreteFIFOSpec("", 1, 1, false, 0)
	}
	return fifoSpec{
		ModuleName:            sanitizedModuleName(decl.ModuleName),
		DataWidth:             normalizedPositive(decl.DataWidth, 1),
		Depth:                 normalizedPositive(decl.Depth, 1),
		AddrWidth:             normalizedPositive(decl.AddrWidth, 1),
		CountWidth:            normalizedPositive(decl.CountWidth, 1),
		AsyncReset:            decl.AsyncReset,
		AlmostFullLevel:       normalizedPositive(decl.AlmostFullLevel, 1),
		AlmostEmptyLevel:      maxInt(decl.AlmostEmptyLevel, 0),
		LastPtrValue:          maxInt(decl.LastPtrValue, 0),
		DepthCountValue:       normalizedPositive(decl.DepthCountValue, normalizedPositive(decl.Depth, 1)),
		AlmostFullCountValue:  normalizedPositive(decl.AlmostFullCountValue, 1),
		AlmostEmptyCountValue: maxInt(decl.AlmostEmptyCountValue, 0),
		UseRegisteredRead:     decl.UseRegisteredRead,
		AlmostEmptyUsesEmpty:  decl.AlmostEmptyUsesEmpty,
	}
}

func renderConcreteFIFOVerilog(spec fifoSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s (\n", spec.ModuleName)
	fmt.Fprintf(&b, "  input  wire                   clk,\n")
	fmt.Fprintf(&b, "  input  wire                   rst_n,\n")
	fmt.Fprintf(&b, "  input  wire                   wr_en,\n")
	fmt.Fprintf(&b, "  input  wire %s          wr_data,\n", fifoDataRange(spec.DataWidth))
	fmt.Fprintf(&b, "  output wire                   full,\n")
	fmt.Fprintf(&b, "  output wire                   almost_full,\n")
	fmt.Fprintf(&b, "  input  wire                   rd_en,\n")
	fmt.Fprintf(&b, "  output wire %s          rd_data,\n", fifoDataRange(spec.DataWidth))
	fmt.Fprintf(&b, "  output wire                   empty,\n")
	fmt.Fprintf(&b, "  output wire                   almost_empty\n")
	fmt.Fprintf(&b, ");\n\n")

	fmt.Fprintf(&b, "  localparam integer DATA_WIDTH = %d;\n", spec.DataWidth)
	fmt.Fprintf(&b, "  localparam integer DEPTH = %d;\n", spec.Depth)
	fmt.Fprintf(&b, "  localparam integer ADDR_WIDTH = %d;\n", spec.AddrWidth)
	fmt.Fprintf(&b, "  localparam integer COUNT_WIDTH = %d;\n", spec.CountWidth)
	fmt.Fprintf(&b, "  localparam integer LAST_PTR_VALUE = %d;\n", spec.LastPtrValue)
	fmt.Fprintf(&b, "  localparam integer DEPTH_COUNT_VALUE = %d;\n", spec.DepthCountValue)
	fmt.Fprintf(&b, "  localparam integer ALMOST_FULL_LEVEL = %d;\n", spec.AlmostFullLevel)
	fmt.Fprintf(&b, "  localparam integer ALMOST_EMPTY_LEVEL = %d;\n", spec.AlmostEmptyLevel)
	fmt.Fprintf(&b, "  localparam integer ALMOST_FULL_COUNT_VALUE = %d;\n", spec.AlmostFullCountValue)
	fmt.Fprintf(&b, "  localparam integer ALMOST_EMPTY_COUNT_VALUE = %d;\n", spec.AlmostEmptyCountValue)
	fmt.Fprintf(&b, "  localparam integer USE_REGISTERED_READ = %d;\n", boolInt(spec.UseRegisteredRead))
	fmt.Fprintf(&b, "  localparam integer ALMOST_EMPTY_USES_EMPTY = %d;\n", boolInt(spec.AlmostEmptyUsesEmpty))
	fmt.Fprintf(&b, "  localparam integer ASYNC_RESET = %d;\n\n", boolInt(spec.AsyncReset))

	fmt.Fprintf(&b, "  %s #(\n", reusableFIFOName)
	fmt.Fprintf(&b, "    .DATA_WIDTH(DATA_WIDTH),\n")
	fmt.Fprintf(&b, "    .DEPTH(DEPTH),\n")
	fmt.Fprintf(&b, "    .ADDR_WIDTH(ADDR_WIDTH),\n")
	fmt.Fprintf(&b, "    .COUNT_WIDTH(COUNT_WIDTH),\n")
	fmt.Fprintf(&b, "    .LAST_PTR_VALUE(LAST_PTR_VALUE),\n")
	fmt.Fprintf(&b, "    .DEPTH_COUNT_VALUE(DEPTH_COUNT_VALUE),\n")
	fmt.Fprintf(&b, "    .ALMOST_FULL_LEVEL(ALMOST_FULL_LEVEL),\n")
	fmt.Fprintf(&b, "    .ALMOST_EMPTY_LEVEL(ALMOST_EMPTY_LEVEL),\n")
	fmt.Fprintf(&b, "    .ALMOST_FULL_COUNT_VALUE(ALMOST_FULL_COUNT_VALUE),\n")
	fmt.Fprintf(&b, "    .ALMOST_EMPTY_COUNT_VALUE(ALMOST_EMPTY_COUNT_VALUE),\n")
	fmt.Fprintf(&b, "    .USE_REGISTERED_READ(USE_REGISTERED_READ),\n")
	fmt.Fprintf(&b, "    .ALMOST_EMPTY_USES_EMPTY(ALMOST_EMPTY_USES_EMPTY),\n")
	fmt.Fprintf(&b, "    .ASYNC_RESET(ASYNC_RESET)\n")
	fmt.Fprintf(&b, "  ) fifo_impl (\n")
	fmt.Fprintf(&b, "    .clk(clk),\n")
	fmt.Fprintf(&b, "    .rst_n(rst_n),\n")
	fmt.Fprintf(&b, "    .wr_en(wr_en),\n")
	fmt.Fprintf(&b, "    .wr_data(wr_data),\n")
	fmt.Fprintf(&b, "    .full(full),\n")
	fmt.Fprintf(&b, "    .almost_full(almost_full),\n")
	fmt.Fprintf(&b, "    .rd_en(rd_en),\n")
	fmt.Fprintf(&b, "    .rd_data(rd_data),\n")
	fmt.Fprintf(&b, "    .empty(empty),\n")
	fmt.Fprintf(&b, "    .almost_empty(almost_empty)\n")
	fmt.Fprintf(&b, "  );\n")
	fmt.Fprintf(&b, "endmodule")
	return b.String()
}

func sanitizedModuleName(name string) string {
	name = sanitize(name)
	if name == "" {
		return reusableFIFOName
	}
	return name
}

func fifoAddrWidth(depth int) int {
	if depth <= 1 {
		return 1
	}
	width := 0
	for value := depth - 1; value > 0; value >>= 1 {
		width++
	}
	if width == 0 {
		return 1
	}
	return width
}

func fifoCountWidth(depth int) int {
	if depth <= 1 {
		return 1
	}
	width := 0
	for value := depth; value > 0; value >>= 1 {
		width++
	}
	if width == 0 {
		return 1
	}
	return width
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

func defaultAlmostEmptyLevel(depth int) int {
	if depth <= 1 {
		return 0
	}
	return 1
}

func fifoDataRange(width int) string {
	if width <= 1 {
		return ""
	}
	return fmt.Sprintf("[%d:0]", width-1)
}

func normalizedPositive(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
