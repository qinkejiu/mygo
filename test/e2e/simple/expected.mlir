module {
  hw.module @main(%clk: i1, %rst: i1) {
    hw.instance "main_inst0" @main__proc_main(%clk, %rst) : (i1, i1) -> ()
    hw.output
  }
  hw.module @main__proc_main(%clk: i1, %rst: i1) {
    %c0 = hw.constant 1 : i32
    %c1 = hw.constant 2 : i16
    %v2 = comb.sext %c0 : i32 to i64
    %v3 = comb.sext %c1 : i16 to i64
    %v4 = comb.add %v2, %v3 : i64
  }
}
