module main(
  input clk,
        rst
);

  wire [31:0] _GEN = 32'h1;
  wire [15:0] _GEN_0 = 16'h2;
  always @(posedge clk)
    $fwrite(32'h80000001, "The result is small: %d\n",
            {{32{_GEN[31]}}, _GEN} + {{48{_GEN_0[15]}}, _GEN_0});
endmodule

