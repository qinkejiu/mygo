module main(
  input clk,
        rst
);

  reg  [2:0]  state_reg7;
  initial
    state_reg7 = 3'h0;
  wire        _GEN = 32'shA > 32'sh3;
  wire signed [31:0] _GEN_0 = 32'sh3 - 32'shA;
  wire signed [31:0] _GEN_1 = 32'shA - 32'sh3;
  always @(posedge clk) begin
    if (rst)
      state_reg7 <= 3'h0;
    else begin
      case (state_reg7)
        3'b000: begin
          if (_GEN)
            state_reg7 <= 3'h2;
          else
            state_reg7 <= 3'h1;
        end
        3'b001: begin
          $fwrite(32'h80000001, "branch y>=x delta=%d (x=%d y=%d)\n", _GEN_0, 32'shA,
                  32'sh3);
          state_reg7 <= 3'h3;
        end
        3'b010: begin
          $fwrite(32'h80000001, "branch x>y delta=%d (x=%d y=%d)\n", _GEN_1, 32'shA,
                  32'sh3);
          state_reg7 <= 3'h3;
        end
        3'b011:
          state_reg7 <= 3'h4;
        3'b100:
          state_reg7 <= state_reg7;
        default:
          state_reg7 <= state_reg7;
      endcase
    end
  end // always @(posedge)
endmodule

