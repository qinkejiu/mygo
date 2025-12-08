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
  mygo_fifo_i32_d4 t0_fifo (
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
  assign chan_t0_wdata = 32'h5;
  assign chan_t0_wvalid = 1'h1;
  assign chan_t1_rready = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "main observed=%d\n", chan_t1_rdata);
  main__proc_worker worker_inst0 (
    .clk            (clk),
    .rst            (rst),
    .chan_t0_rdata  (chan_t0_rdata),
    .chan_t0_rvalid (chan_t0_rvalid),
    .chan_t0_rready (chan_t0_rready),
    .chan_t1_wdata  (chan_t1_wdata),
    .chan_t1_wvalid (chan_t1_wvalid),
    .chan_t1_wready (chan_t1_wready)
  );
endmodule

module main__proc_worker(
  input        clk,
               rst,
  inout [31:0] chan_t0_rdata,
  inout        chan_t0_rvalid,
               chan_t0_rready,
  inout [31:0] chan_t1_wdata,
  inout        chan_t1_wvalid,
               chan_t1_wready
);

  assign chan_t0_rready = 1'h1;
  always @(posedge clk)
    $fwrite(32'h80000001, "worker received=%d produced=%d\n", chan_t0_rdata,
            chan_t1_wdata);
  assign chan_t1_wdata = chan_t0_rdata + 32'h1;
  assign chan_t1_wvalid = 1'h1;
endmodule

