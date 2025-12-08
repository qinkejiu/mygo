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
  assign chan_t1_rready = 1'h1;
  assign chan_t0_wdata = chan_t1_rdata;
  assign chan_t0_wvalid = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "phi loop final=%d\n", chan_t1_rdata);
  main__proc_consumer consumer_inst0 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_rdata  (chan_t0_rdata),
    .chan_t0_rvalid (chan_t0_rvalid),
    .chan_t0_rready (chan_t0_rready),
    .chan_t1_wdata  (chan_t1_wdata),
    .chan_t1_wvalid (chan_t1_wvalid),
    .chan_t1_wready (chan_t1_wready)
  );
  main__proc_producer producer_inst1 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_wdata  (chan_t0_wdata),
    .chan_t0_wvalid (chan_t0_wvalid),
    .chan_t0_wready (chan_t0_wready)
  );
endmodule

module main__proc_consumer(
  input        clk,
               rst,
  inout [31:0] chan_t0_rdata,
  inout        chan_t0_rvalid,
               chan_t0_rready,
  inout [31:0] chan_t1_wdata,
  inout        chan_t1_wvalid,
               chan_t1_wready
);

  reg  [2:0]  state_reg9;
  initial
    state_reg9 = 3'h0;
  reg  [31:0] phi_reg11;
  reg  [31:0] phi_reg13;
  assign chan_t1_wdata = phi_reg11;
  assign chan_t1_wvalid = 1'h1;
  assign chan_t0_rready = 1'h1;
  wire [31:0] _GEN = phi_reg11 + chan_t0_rdata;
  always @(posedge clk)
    $fwrite(32'h80000001, "consumer received %d (running total %d)\n", chan_t0_rdata,
            _GEN);
  always @(posedge clk) begin
    case (state_reg9)
      3'b000: begin
        state_reg9 <= 3'h1;
        phi_reg11 <= 32'h0;
        phi_reg13 <= 32'h0;
      end
      3'b001: begin
        if ($signed(phi_reg13) < 32'sh4)
          state_reg9 <= 3'h3;
        else
          state_reg9 <= 3'h2;
      end
      3'b010:
        state_reg9 <= 3'h4;
      3'b011: begin
        state_reg9 <= 3'h1;
        phi_reg11 <= _GEN;
        phi_reg13 <= phi_reg13 + 32'h1;
      end
      3'b100:
        state_reg9 <= state_reg9;
      default:
        state_reg9 <= state_reg9;
    endcase
  end // always @(posedge)
endmodule

module main__proc_producer(
  input        clk,
               rst,
  inout [31:0] chan_t0_wdata,
  inout        chan_t0_wvalid,
               chan_t0_wready
);

  reg [2:0]  state_reg8;
  initial
    state_reg8 = 3'h0;
  reg [31:0] phi_reg10;
  assign chan_t0_wdata = phi_reg10;
  assign chan_t0_wvalid = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "producer sent %d\n", phi_reg10);
  always @(posedge clk) begin
    case (state_reg8)
      3'b000: begin
        state_reg8 <= 3'h1;
        phi_reg10 <= 32'h0;
      end
      3'b001: begin
        if ($signed(phi_reg10) < 32'sh4)
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

