module main(
  input clk,
        rst
);
  reg fired = 0;
  always @(posedge clk) begin
    if (rst) begin
      fired <= 0;
    end else if (!fired) begin
      fired <= 1;
      $fwrite(32'h80000001, "verilator trace=42\n");
    end
  end
endmodule
module mygo_fifo_i32_d1();
endmodule
module mygo_fifo_i32_d4();
endmodule
module mygo_fifo_i1_d1();
endmodule
