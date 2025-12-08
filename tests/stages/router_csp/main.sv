module main(
  input clk,
        rst
);

  wire [31:0] chan_t0_wdata;
  wire        chan_t0_wvalid;
  wire        chan_t0_wready;
  wire [31:0] chan_t0_rdata;
  wire        chan_t0_rvalid;
  wire        chan_t0_rready;
  wire [31:0] chan_t1_wdata;
  wire        chan_t1_wvalid;
  wire        chan_t1_wready;
  wire [31:0] chan_t1_rdata;
  wire        chan_t1_rvalid;
  wire        chan_t1_rready;
  wire [31:0] chan_t2_wdata;
  wire        chan_t2_wvalid;
  wire        chan_t2_wready;
  wire [31:0] chan_t2_rdata;
  wire        chan_t2_rvalid;
  wire        chan_t2_rready;
  wire [31:0] chan_t3_wdata;
  wire        chan_t3_wvalid;
  wire        chan_t3_wready;
  wire [31:0] chan_t3_rdata;
  wire        chan_t3_rvalid;
  wire        chan_t3_rready;
  wire        chan_t4_wdata;
  wire        chan_t4_wvalid;
  wire        chan_t4_wready;
  wire        chan_t4_rdata;
  wire        chan_t4_rvalid;
  wire        chan_t4_rready;
  mygo_fifo_i32_d1 t0_fifo (
    .clk       (clk),
    .rst       (rst),
    .in_data   (chan_t0_wdata),
    .in_valid  (chan_t0_wvalid),
    .in_ready  (chan_t0_wready),
    .out_data  (chan_t0_rdata),
    .out_valid (chan_t0_rvalid),
    .out_ready (chan_t0_rready)
  );
  mygo_fifo_i32_d1 t1_fifo (
    .clk       (clk),
    .rst       (rst),
    .in_data   (chan_t1_wdata),
    .in_valid  (chan_t1_wvalid),
    .in_ready  (chan_t1_wready),
    .out_data  (chan_t1_rdata),
    .out_valid (chan_t1_rvalid),
    .out_ready (chan_t1_rready)
  );
  mygo_fifo_i32_d1 t2_fifo (
    .clk       (clk),
    .rst       (rst),
    .in_data   (chan_t2_wdata),
    .in_valid  (chan_t2_wvalid),
    .in_ready  (chan_t2_wready),
    .out_data  (chan_t2_rdata),
    .out_valid (chan_t2_rvalid),
    .out_ready (chan_t2_rready)
  );
  mygo_fifo_i32_d1 t3_fifo (
    .clk       (clk),
    .rst       (rst),
    .in_data   (chan_t3_wdata),
    .in_valid  (chan_t3_wvalid),
    .in_ready  (chan_t3_wready),
    .out_data  (chan_t3_rdata),
    .out_valid (chan_t3_rvalid),
    .out_ready (chan_t3_rready)
  );
  mygo_fifo_i1_d2 t4_fifo (
    .clk       (clk),
    .rst       (rst),
    .in_data   (chan_t4_wdata),
    .in_valid  (chan_t4_wvalid),
    .in_ready  (chan_t4_wready),
    .out_data  (chan_t4_rdata),
    .out_valid (chan_t4_rvalid),
    .out_ready (chan_t4_rready)
  );
  reg  [2:0]  state_reg14;
  initial
    state_reg14 = 3'h0;
  reg  [31:0] phi_reg16;
  assign chan_t4_rready = 1'h1;
  always @(posedge clk) begin
    case (state_reg14)
      3'b000: begin
        state_reg14 <= 3'h1;
        phi_reg16 <= 32'h0;
      end
      3'b001: begin
        if ($signed(phi_reg16) < 32'sh2)
          state_reg14 <= 3'h3;
        else
          state_reg14 <= 3'h2;
      end
      3'b010:
        state_reg14 <= 3'h4;
      3'b011: begin
        state_reg14 <= 3'h1;
        phi_reg16 <= phi_reg16 + 32'h1;
      end
      3'b100:
        state_reg14 <= state_reg14;
      default:
        state_reg14 <= state_reg14;
    endcase
  end // always @(posedge)
  main__proc_consumer consumer_inst0 (
    .clk            (clk),
    .rst            (rst),
    .chan_t2_rdata  (chan_t2_rdata),
    .chan_t2_rvalid (chan_t2_rvalid),
    .chan_t2_rready (chan_t2_rready),
    .chan_t4_wdata  (chan_t4_wdata),
    .chan_t4_wvalid (chan_t4_wvalid),
    .chan_t4_wready (chan_t4_wready)
  );
  main__proc_producer producer_inst1 (
    .clk (clk),
    .rst (rst)
  );
  main__proc_router router_inst2 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_rdata  (chan_t0_rdata),
    .chan_t0_rvalid (chan_t0_rvalid),
    .chan_t0_rready (chan_t0_rready),
    .chan_t1_rdata  (chan_t1_rdata),
    .chan_t1_rvalid (chan_t1_rvalid),
    .chan_t1_rready (chan_t1_rready)
  );
endmodule

module main__proc_consumer(
  input        clk,
               rst,
  inout [31:0] chan_t2_rdata,
  inout        chan_t2_rvalid,
               chan_t2_rready,
               chan_t4_wdata,
               chan_t4_wvalid,
               chan_t4_wready
);

  reg [2:0]  state_reg9;
  initial
    state_reg9 = 3'h0;
  reg [31:0] phi_reg11;
  assign chan_t4_wdata = 1'h1;
  assign chan_t4_wvalid = 1'h1;
  assign chan_t2_rready = 1'h1;
  always @(posedge clk) begin
    case (state_reg9)
      3'b000: begin
        state_reg9 <= 3'h1;
        phi_reg11 <= 32'h0;
      end
      3'b001: begin
        if (phi_reg11 < 32'h4)
          state_reg9 <= 3'h3;
        else
          state_reg9 <= 3'h2;
      end
      3'b010:
        state_reg9 <= 3'h4;
      3'b011: begin
        state_reg9 <= 3'h1;
        phi_reg11 <= phi_reg11 + 32'h1;
      end
      3'b100:
        state_reg9 <= state_reg9;
      default:
        state_reg9 <= state_reg9;
    endcase
  end // always @(posedge)
endmodule

module main__proc_producer(
  input clk,
        rst
);

  reg [2:0]  state_reg13;
  initial
    state_reg13 = 3'h0;
  reg [31:0] phi_reg15;
  always @(posedge clk)
    $fwrite(32'h80000001, "producer %d sent dest=%d payload=%d\n", 32'h0,
            32'h0 + phi_reg15 & 32'h1, 32'h0 * 32'hA + phi_reg15);
  always @(posedge clk) begin
    case (state_reg13)
      3'b000: begin
        state_reg13 <= 3'h1;
        phi_reg15 <= 32'h0;
      end
      3'b001: begin
        if (phi_reg15 < 32'h4)
          state_reg13 <= 3'h3;
        else
          state_reg13 <= 3'h2;
      end
      3'b010:
        state_reg13 <= 3'h4;
      3'b011: begin
        state_reg13 <= 3'h1;
        phi_reg15 <= phi_reg15 + 32'h1;
      end
      3'b100:
        state_reg13 <= state_reg13;
      default:
        state_reg13 <= state_reg13;
    endcase
  end // always @(posedge)
endmodule

module main__proc_router(
  input        clk,
               rst,
  inout [31:0] chan_t0_rdata,
  inout        chan_t0_rvalid,
               chan_t0_rready,
  inout [31:0] chan_t1_rdata,
  inout        chan_t1_rvalid,
               chan_t1_rready
);

  reg [2:0]  state_reg8;
  initial
    state_reg8 = 3'h0;
  reg [31:0] phi_reg10;
  assign chan_t0_rready = 1'h1;
  assign chan_t1_rready = 1'h1;
  always @(posedge clk) begin
    case (state_reg8)
      3'b000: begin
        state_reg8 <= 3'h1;
        phi_reg10 <= 32'h0;
      end
      3'b001: begin
        if (phi_reg10 < 32'h4)
          state_reg8 <= 3'h3;
        else
          state_reg8 <= 3'h2;
      end
      3'b010:
        state_reg8 <= 3'h4;
      3'b011: begin
        state_reg8 <= 3'h1;
        phi_reg10 <= phi_reg10 + 32'h1;
      end
      3'b100:
        state_reg8 <= state_reg8;
      default:
        state_reg8 <= state_reg8;
    endcase
  end // always @(posedge)
endmodule

