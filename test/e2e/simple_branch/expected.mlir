module {
  hw.module @main(%clk: i1, %rst: i1) {
    hw.instance "main_inst0" @main__proc_main(%clk, %rst) : (i1, i1) -> ()
    hw.output
  }
  hw.module @main__proc_main(%clk: i1, %rst: i1) {
    %c0 = hw.constant 10 : i32
    %c1 = hw.constant 3 : i32
    %v2 = comb.icmp sgt, %c0, %c1 : i32
    %v3 = comb.sub %c1, %c0 : i32
    %v4 = comb.sub %c0, %c1 : i32
    %v5 = comb.mux %v2, %v4, %v3 : i1, i32
  }
}
