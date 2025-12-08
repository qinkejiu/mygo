module main(
  input clk,
        rst
);

  wire [31:0] _GEN = 32'h3 - 32'hA;
  always @(posedge clk)
    $fwrite(32'h80000001, "branch y>=x delta=%d (x=%d y=%d)\n", _GEN, 32'hA, 32'h3);
  wire [31:0] _GEN_0 = 32'hA - 32'h3;
  always @(posedge clk)
    $fwrite(32'h80000001, "branch x>y delta=%d (x=%d y=%d)\n", _GEN_0, 32'hA, 32'h3);
endmodule

