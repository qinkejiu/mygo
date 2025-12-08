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
  wire [7:0]  chan_t1_wdata;
  wire        chan_t1_wvalid;
  wire        chan_t1_wready;
  wire [7:0]  chan_t1_rdata;
  wire        chan_t1_rvalid;
  wire        chan_t1_rready;
  wire        chan_t2_wdata;
  wire        chan_t2_wvalid;
  wire        chan_t2_wready;
  wire        chan_t2_rdata;
  wire        chan_t2_rvalid;
  wire        chan_t2_rready;
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
  mygo_fifo_i8_d8 t1_fifo (
    .clk       (clk),
    .rst       (rst),
    .in_data   (chan_t1_wdata),
    .in_valid  (chan_t1_wvalid),
    .in_ready  (chan_t1_wready),
    .out_data  (chan_t1_rdata),
    .out_valid (chan_t1_rvalid),
    .out_ready (chan_t1_rready)
  );
  mygo_fifo_i1_d1 t2_fifo (
    .clk       (clk),
    .rst       (rst),
    .in_data   (chan_t2_wdata),
    .in_valid  (chan_t2_wvalid),
    .in_ready  (chan_t2_wready),
    .out_data  (chan_t2_rdata),
    .out_valid (chan_t2_rvalid),
    .out_ready (chan_t2_rready)
  );
  assign chan_t2_rready = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "finished is %d\n", chan_t2_rdata);
  main__proc_stage1 stage1_inst0 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_wdata  (chan_t0_wdata),
    .chan_t0_wvalid (chan_t0_wvalid),
    .chan_t0_wready (chan_t0_wready)
  );
  main__proc_stage2 stage2_inst1 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_rdata  (chan_t0_rdata),
    .chan_t0_rvalid (chan_t0_rvalid),
    .chan_t0_rready (chan_t0_rready)
  );
  main__proc_stage3 stage3_inst2 (
    .clk            (clk),
    .rst            (rst),
    .chan_t1_rdata  (chan_t1_rdata),
    .chan_t1_rvalid (chan_t1_rvalid),
    .chan_t1_rready (chan_t1_rready),
    .chan_t2_wdata  (chan_t2_wdata),
    .chan_t2_wvalid (chan_t2_wvalid),
    .chan_t2_wready (chan_t2_wready)
  );
endmodule

module main__proc_stage1(
  input        clk,
               rst,
  inout [31:0] chan_t0_wdata,
  inout        chan_t0_wvalid,
               chan_t0_wready
);

  reg [2:0]  state_reg9;
  initial
    state_reg9 = 3'h0;
  assign chan_t0_wdata = 32'h4;
  assign chan_t0_wvalid = 1'h1;
  reg [31:0] phi_reg11;
  assign chan_t0_wdata = phi_reg11 + phi_reg11;
  assign chan_t0_wvalid = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "stage 1: sent integer %d\n", chan_t0_wdata);
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

module main__proc_stage2(
  input        clk,
               rst,
  inout [31:0] chan_t0_rdata,
  inout        chan_t0_rvalid,
               chan_t0_rready
);

  reg [2:0]  state_reg8;
  initial
    state_reg8 = 3'h0;
  assign chan_t0_rready = 1'h1;
  reg [31:0] phi_reg11;
  assign chan_t0_rready = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "stage 2: emitted 4 bytes for %d\n", chan_t0_rdata);
  always @(posedge clk) begin
    case (state_reg8)
      3'b000: begin
        state_reg8 <= 3'h1;
        phi_reg11 <= 32'h0;
      end
      3'b001: begin
        if (phi_reg11 < 32'h4)
          state_reg8 <= 3'h3;
        else
          state_reg8 <= 3'h2;
      end
      3'b010:
        state_reg8 <= 3'h4;
      3'b011: begin
        state_reg8 <= 3'h1;
        phi_reg11 <= phi_reg11 + 32'h1;
      end
      3'b100:
        state_reg8 <= state_reg8;
      default:
        state_reg8 <= state_reg8;
    endcase
  end // always @(posedge)
endmodule

module main__proc_stage3(
  input       clk,
              rst,
  inout [7:0] chan_t1_rdata,
  inout       chan_t1_rvalid,
              chan_t1_rready,
              chan_t2_wdata,
              chan_t2_wvalid,
              chan_t2_wready
);

  reg [2:0]  state_reg12;
  initial
    state_reg12 = 3'h0;
  reg [31:0] phi_reg14;
  assign chan_t2_wdata = 1'h1;
  assign chan_t2_wvalid = 1'h1;
  assign chan_t1_rready = 1'h1;
  assign chan_t1_rready = 1'h1;
  assign chan_t1_rready = 1'h1;
  assign chan_t1_rready = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "stage 3: reconstructed integer %d\n",
            {24'h0, chan_t1_rdata} << 32'h18 | {24'h0, chan_t1_rdata} << 32'h10
            | {24'h0, chan_t1_rdata} << 32'h8 | {24'h0, chan_t1_rdata});
  always @(posedge clk) begin
    case (state_reg12)
      3'b000: begin
        state_reg12 <= 3'h1;
        phi_reg14 <= 32'h0;
      end
      3'b001: begin
        if (phi_reg14 < 32'h4)
          state_reg12 <= 3'h3;
        else
          state_reg12 <= 3'h2;
      end
      3'b010:
        state_reg12 <= 3'h4;
      3'b011: begin
        state_reg12 <= 3'h1;
        phi_reg14 <= phi_reg14 + 32'h1;
      end
      3'b100:
        state_reg12 <= state_reg12;
      default:
        state_reg12 <= state_reg12;
    endcase
  end // always @(posedge)
endmodule

