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
  mygo_fifo_i32_d4 t1_fifo (
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
  main__proc_filter filter_inst0 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_rdata  (chan_t0_rdata),
    .chan_t0_rvalid (chan_t0_rvalid),
    .chan_t0_rready (chan_t0_rready),
    .chan_t1_wdata  (chan_t1_wdata),
    .chan_t1_wvalid (chan_t1_wvalid),
    .chan_t1_wready (chan_t1_wready)
  );
  main__proc_sink sink_inst1 (
    .clk            (clk),
    .rst            (rst),
    .chan_t1_rdata  (chan_t1_rdata),
    .chan_t1_rvalid (chan_t1_rvalid),
    .chan_t1_rready (chan_t1_rready),
    .chan_t2_wdata  (chan_t2_wdata),
    .chan_t2_wvalid (chan_t2_wvalid),
    .chan_t2_wready (chan_t2_wready)
  );
  main__proc_source source_inst2 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_wdata  (chan_t0_wdata),
    .chan_t0_wvalid (chan_t0_wvalid),
    .chan_t0_wready (chan_t0_wready)
  );
endmodule

module main__proc_filter(
  input        clk,
               rst,
  inout [31:0] chan_t0_rdata,
  inout        chan_t0_rvalid,
               chan_t0_rready,
  inout [31:0] chan_t1_wdata,
  inout        chan_t1_wvalid,
               chan_t1_wready
);

  reg [3:0]  state_reg17;
  initial
    state_reg17 = 4'h0;
  assign chan_t0_rready = 1'h1;
  assign chan_t1_wdata = 32'h5;
  assign chan_t1_wvalid = 1'h1;
  reg [31:0] phi_reg20;
  assign chan_t0_rready = 1'h1;
  reg [31:0] phi_reg26;
  assign chan_t1_wdata = phi_reg26;
  assign chan_t1_wvalid = 1'h1;
  always @(posedge clk) begin
    case (state_reg17)
      4'b0000: begin
        state_reg17 <= 4'h1;
        phi_reg20 <= 32'h0;
      end
      4'b0001: begin
        if (phi_reg20 < 32'h5)
          state_reg17 <= 4'h3;
        else
          state_reg17 <= 4'h2;
      end
      4'b0010:
        state_reg17 <= 4'h8;
      4'b0011: begin
        if (chan_t0_rdata == 32'h19700328)
          state_reg17 <= 4'h6;
        else
          state_reg17 <= 4'h4;
      end
      4'b0100: begin
        if (chan_t0_rdata == 32'h19700101)
          state_reg17 <= 4'h5;
        else begin
          state_reg17 <= 4'h7;
          phi_reg26 <= chan_t0_rdata;
        end
      end
      4'b0101: begin
        state_reg17 <= 4'h7;
        phi_reg26 <= 32'h20071224;
      end
      4'b0110: begin
        state_reg17 <= 4'h7;
        phi_reg26 <= 32'h20050823;
      end
      4'b0111: begin
        state_reg17 <= 4'h1;
        phi_reg20 <= phi_reg20 + 32'h1;
      end
      4'b1000:
        state_reg17 <= state_reg17;
      default:
        state_reg17 <= state_reg17;
    endcase
  end // always @(posedge)
endmodule

module main__proc_sink(
  input        clk,
               rst,
  inout [31:0] chan_t1_rdata,
  inout        chan_t1_rvalid,
               chan_t1_rready,
               chan_t2_wdata,
               chan_t2_wvalid,
               chan_t2_wready
);

  reg [2:0]  state_reg9;
  initial
    state_reg9 = 3'h0;
  assign chan_t1_rready = 1'h1;
  reg [31:0] phi_reg12;
  assign chan_t2_wdata = 1'h1;
  assign chan_t2_wvalid = 1'h1;
  assign chan_t1_rready = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "output: count %d got integer 0x%x\n", phi_reg12,
            chan_t1_rdata);
  always @(posedge clk) begin
    case (state_reg9)
      3'b000: begin
        state_reg9 <= 3'h1;
        phi_reg12 <= 32'h0;
      end
      3'b001: begin
        if (phi_reg12 < 32'h5)
          state_reg9 <= 3'h3;
        else
          state_reg9 <= 3'h2;
      end
      3'b010:
        state_reg9 <= 3'h4;
      3'b011: begin
        state_reg9 <= 3'h1;
        phi_reg12 <= phi_reg12 + 32'h1;
      end
      3'b100:
        state_reg9 <= state_reg9;
      default:
        state_reg9 <= state_reg9;
    endcase
  end // always @(posedge)
endmodule

module main__proc_source(
  input        clk,
               rst,
  inout [31:0] chan_t0_wdata,
  inout        chan_t0_wvalid,
               chan_t0_wready
);

  reg [3:0]  state_reg17;
  initial
    state_reg17 = 4'h0;
  assign chan_t0_wdata = 32'h5;
  assign chan_t0_wvalid = 1'h1;
  reg [31:0] phi_reg19;
  reg [31:0] phi_reg24;
  assign chan_t0_wdata = phi_reg24;
  assign chan_t0_wvalid = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "input: count %d sent integer 0x%x\n", phi_reg19, phi_reg24);
  always @(posedge clk) begin
    case (state_reg17)
      4'b0000: begin
        state_reg17 <= 4'h1;
        phi_reg19 <= 32'h0;
      end
      4'b0001: begin
        if (phi_reg19 < 32'h5)
          state_reg17 <= 4'h3;
        else
          state_reg17 <= 4'h2;
      end
      4'b0010:
        state_reg17 <= 4'h8;
      4'b0011: begin
        if (phi_reg19 == 32'h1)
          state_reg17 <= 4'h6;
        else
          state_reg17 <= 4'h4;
      end
      4'b0100: begin
        if (phi_reg19 == 32'h2)
          state_reg17 <= 4'h5;
        else begin
          state_reg17 <= 4'h7;
          phi_reg24 <= phi_reg19;
        end
      end
      4'b0101: begin
        state_reg17 <= 4'h7;
        phi_reg24 <= 32'h19700101;
      end
      4'b0110: begin
        state_reg17 <= 4'h7;
        phi_reg24 <= 32'h19700328;
      end
      4'b0111: begin
        state_reg17 <= 4'h1;
        phi_reg19 <= phi_reg19 + 32'h1;
      end
      4'b1000:
        state_reg17 <= state_reg17;
      default:
        state_reg17 <= state_reg17;
    endcase
  end // always @(posedge)
endmodule

