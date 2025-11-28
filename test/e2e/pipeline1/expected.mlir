module {
  hw.module @main(%clk: i1, %rst: i1) {
    // channel t0 depth=1 type=i32
    %chan_t0_wdata = sv.wire : !hw.inout<i32>
    %chan_t0_wvalid = sv.wire : !hw.inout<i1>
    %chan_t0_wready = sv.wire : !hw.inout<i1>
    %chan_t0_rdata = sv.wire : !hw.inout<i32>
    %chan_t0_rvalid = sv.wire : !hw.inout<i1>
    %chan_t0_rready = sv.wire : !hw.inout<i1>
    // channel t0 occupancy 1/1
    //   producer source stage 3
    //   producer source stage 3
    //   consumer filter stage 2
    //   consumer filter stage 2
    // channel t1 depth=4 type=i32
    %chan_t1_wdata = sv.wire : !hw.inout<i32>
    %chan_t1_wvalid = sv.wire : !hw.inout<i1>
    %chan_t1_wready = sv.wire : !hw.inout<i1>
    %chan_t1_rdata = sv.wire : !hw.inout<i32>
    %chan_t1_rvalid = sv.wire : !hw.inout<i1>
    %chan_t1_rready = sv.wire : !hw.inout<i1>
    // channel t1 occupancy 2/4
    //   producer filter stage 2
    //   producer filter stage 2
    //   consumer sink stage 1
    //   consumer sink stage 1
    // channel t2 depth=1 type=i1
    %chan_t2_wdata = sv.wire : !hw.inout<i1>
    %chan_t2_wvalid = sv.wire : !hw.inout<i1>
    %chan_t2_wready = sv.wire : !hw.inout<i1>
    %chan_t2_rdata = sv.wire : !hw.inout<i1>
    %chan_t2_rvalid = sv.wire : !hw.inout<i1>
    %chan_t2_rready = sv.wire : !hw.inout<i1>
    // channel t2 occupancy 0/1
    //   producer sink stage 1
    //   consumer main stage 0
    hw.instance "t0_fifo" @mygo.fifo_i32_d1(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t1_fifo" @mygo.fifo_i32_d4(%clk, %rst, %chan_t1_wdata, %chan_t1_wvalid, %chan_t1_wready, %chan_t1_rdata, %chan_t1_rvalid, %chan_t1_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t2_fifo" @mygo.fifo_i1_d1(%clk, %rst, %chan_t2_wdata, %chan_t2_wvalid, %chan_t2_wready, %chan_t2_rdata, %chan_t2_rvalid, %chan_t2_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "filter_inst0" @main__proc_filter(%clk, %rst, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready, %chan_t1_wdata, %chan_t1_wvalid, %chan_t1_wready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "main_inst1" @main__proc_main(%clk, %rst, %chan_t2_rdata, %chan_t2_rvalid, %chan_t2_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "sink_inst2" @main__proc_sink(%clk, %rst, %chan_t1_rdata, %chan_t1_rvalid, %chan_t1_rready, %chan_t2_wdata, %chan_t2_wvalid, %chan_t2_wready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "source_inst3" @main__proc_source(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.output
  }
  hw.module @main__proc_filter(%clk: i1, %rst: i1, %chan_t0_rdata: !hw.inout<i32>, %chan_t0_rvalid: !hw.inout<i1>, %chan_t0_rready: !hw.inout<i1>, %chan_t1_wdata: !hw.inout<i32>, %chan_t1_wvalid: !hw.inout<i1>, %chan_t1_wready: !hw.inout<i1>) {
    %c0 = hw.constant 5 : i32
    %c1 = hw.constant 0 : i32
    %c2 = hw.constant 5 : i32
    %c3 = hw.constant 426771240 : i32
    %c4 = hw.constant 537200675 : i32
    %c5 = hw.constant 537334308 : i32
    %c6 = hw.constant 1 : i32
    %c7 = hw.constant 426770689 : i32
    %v8 = sv.read_inout %chan_t0_rdata : !hw.inout<i32>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_rready, %c_bool_0 : !hw.inout<i1>, i1
    sv.assign %chan_t1_wdata, %c0 : !hw.inout<i32>, i32
    sv.assign %chan_t1_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    // phi t1_11 has 2 incoming values
    %v9 = comb.icmp ult, %t1_11, %c2 : i32
    %v10 = sv.read_inout %chan_t0_rdata : !hw.inout<i32>
    sv.assign %chan_t0_rready, %c_bool_0 : !hw.inout<i1>, i1
    %v11 = comb.icmp eq, %v10, %c3 : i32
    %v12 = comb.icmp eq, %v10, %c7 : i32
    // phi t5_19 has 3 incoming values
    sv.assign %chan_t1_wdata, %t5_19 : !hw.inout<i32>, i32
    sv.assign %chan_t1_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v13 = comb.add %t1_11, %c6 : i32
  }
  hw.module @main__proc_main(%clk: i1, %rst: i1, %chan_t2_rdata: !hw.inout<i1>, %chan_t2_rvalid: !hw.inout<i1>, %chan_t2_rready: !hw.inout<i1>) {
    // spawn sink stage=1 parent_stage=0
    // spawn filter stage=2 parent_stage=0
    // spawn source stage=3 parent_stage=0
    %v0 = sv.read_inout %chan_t2_rdata : !hw.inout<i1>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t2_rready, %c_bool_0 : !hw.inout<i1>, i1
  }
  hw.module @main__proc_sink(%clk: i1, %rst: i1, %chan_t1_rdata: !hw.inout<i32>, %chan_t1_rvalid: !hw.inout<i1>, %chan_t1_rready: !hw.inout<i1>, %chan_t2_wdata: !hw.inout<i1>, %chan_t2_wvalid: !hw.inout<i1>, %chan_t2_wready: !hw.inout<i1>) {
    %c0 = hw.constant 0 : i32
    %c1 = hw.constant 5 : i32
    %c2 = hw.constant 1 : i32
    %c3 = hw.constant true : i1
    %v4 = sv.read_inout %chan_t1_rdata : !hw.inout<i32>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t1_rready, %c_bool_0 : !hw.inout<i1>, i1
    // phi t1_1 has 2 incoming values
    %v5 = comb.icmp ult, %t1_1, %c1 : i32
    sv.assign %chan_t2_wdata, %c3 : !hw.inout<i1>, i1
    sv.assign %chan_t2_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v6 = sv.read_inout %chan_t1_rdata : !hw.inout<i32>
    sv.assign %chan_t1_rready, %c_bool_0 : !hw.inout<i1>, i1
    %v7 = comb.add %t1_1, %c2 : i32
  }
  hw.module @main__proc_source(%clk: i1, %rst: i1, %chan_t0_wdata: !hw.inout<i32>, %chan_t0_wvalid: !hw.inout<i1>, %chan_t0_wready: !hw.inout<i1>) {
    %c0 = hw.constant 5 : i32
    %c1 = hw.constant 0 : i32
    %c2 = hw.constant 5 : i32
    %c3 = hw.constant 1 : i32
    %c4 = hw.constant 426771240 : i32
    %c5 = hw.constant 426770689 : i32
    %c6 = hw.constant 1 : i32
    %c7 = hw.constant 2 : i32
    sv.assign %chan_t0_wdata, %c0 : !hw.inout<i32>, i32
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    // phi t0_26 has 2 incoming values
    %v8 = comb.icmp ult, %t0_26, %c2 : i32
    %v9 = comb.icmp eq, %t0_26, %c3 : i32
    %v10 = comb.icmp eq, %t0_26, %c7 : i32
    // phi t3_33 has 3 incoming values
    sv.assign %chan_t0_wdata, %t3_33 : !hw.inout<i32>, i32
    sv.assign %chan_t0_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v11 = comb.add %t0_26, %c6 : i32
  }
  hw.module @mygo.fifo_i1_d1(%clk: i1, %rst: i1, %in_data: !hw.inout<i1>, %in_valid: !hw.inout<i1>, %in_ready: !hw.inout<i1>, %out_data: !hw.inout<i1>, %out_valid: !hw.inout<i1>, %out_ready: !hw.inout<i1>) {
    // simple passthrough FIFO depth=1
    %write_data = sv.read_inout %in_data : !hw.inout<i1>
    %write_valid = sv.read_inout %in_valid : !hw.inout<i1>
    sv.assign %out_data, %write_data : !hw.inout<i1>, i1
    sv.assign %out_valid, %write_valid : !hw.inout<i1>, i1
    %const_ready = hw.constant true : i1
    sv.assign %in_ready, %const_ready : !hw.inout<i1>, i1
  }
  hw.module @mygo.fifo_i32_d1(%clk: i1, %rst: i1, %in_data: !hw.inout<i32>, %in_valid: !hw.inout<i1>, %in_ready: !hw.inout<i1>, %out_data: !hw.inout<i32>, %out_valid: !hw.inout<i1>, %out_ready: !hw.inout<i1>) {
    // simple passthrough FIFO depth=1
    %write_data = sv.read_inout %in_data : !hw.inout<i32>
    %write_valid = sv.read_inout %in_valid : !hw.inout<i1>
    sv.assign %out_data, %write_data : !hw.inout<i32>, i32
    sv.assign %out_valid, %write_valid : !hw.inout<i1>, i1
    %const_ready = hw.constant true : i1
    sv.assign %in_ready, %const_ready : !hw.inout<i1>, i1
  }
  hw.module @mygo.fifo_i32_d4(%clk: i1, %rst: i1, %in_data: !hw.inout<i32>, %in_valid: !hw.inout<i1>, %in_ready: !hw.inout<i1>, %out_data: !hw.inout<i32>, %out_valid: !hw.inout<i1>, %out_ready: !hw.inout<i1>) {
    // simple passthrough FIFO depth=4
    %write_data = sv.read_inout %in_data : !hw.inout<i32>
    %write_valid = sv.read_inout %in_valid : !hw.inout<i1>
    sv.assign %out_data, %write_data : !hw.inout<i32>, i32
    sv.assign %out_valid, %write_valid : !hw.inout<i1>, i1
    %const_ready = hw.constant true : i1
    sv.assign %in_ready, %const_ready : !hw.inout<i1>, i1
  }
}
