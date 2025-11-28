module {
  hw.module @main(%clk: i1, %rst: i1) {
    hw.instance "main_inst0" @main__proc_main(%clk, %rst) : (i1, i1) -> ()
    hw.output
  }
  hw.module @main__proc_main(%clk: i1, %rst: i1) {
    %c0 = hw.constant 5 : i8
    %c1 = hw.constant -12 : i16
    %c2 = hw.constant 1024 : i32
    %v3 = comb.zext %c0 : i8 to i16
    %v4 = comb.add %v3, %c1 : i16
    %v5 = comb.sext %v4 : i16 to i32
    %v6 = comb.add %v5, %c2 : i32
  }
}
