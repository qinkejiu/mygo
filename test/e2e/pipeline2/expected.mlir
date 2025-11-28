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
    //   producer stage1 stage 3
    //   producer stage1 stage 3
    //   consumer stage2 stage 2
    //   consumer stage2 stage 2
    // channel t1 depth=8 type=i8
    %chan_t1_wdata = sv.wire : !hw.inout<i8>
    %chan_t1_wvalid = sv.wire : !hw.inout<i1>
    %chan_t1_wready = sv.wire : !hw.inout<i1>
    %chan_t1_rdata = sv.wire : !hw.inout<i8>
    %chan_t1_rvalid = sv.wire : !hw.inout<i1>
    %chan_t1_rready = sv.wire : !hw.inout<i1>
    // channel t1 occupancy 0/8
    // channel t2 depth=1 type=i1
    %chan_t2_wdata = sv.wire : !hw.inout<i1>
    %chan_t2_wvalid = sv.wire : !hw.inout<i1>
    %chan_t2_wready = sv.wire : !hw.inout<i1>
    %chan_t2_rdata = sv.wire : !hw.inout<i1>
    %chan_t2_rvalid = sv.wire : !hw.inout<i1>
    %chan_t2_rready = sv.wire : !hw.inout<i1>
    // channel t2 occupancy 0/1
    //   producer stage3 stage 1
    //   consumer main stage 0
    hw.instance "t0_fifo" @mygo.fifo_i32_d1(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t1_fifo" @mygo.fifo_i8_d8(%clk, %rst, %chan_t1_wdata, %chan_t1_wvalid, %chan_t1_wready, %chan_t1_rdata, %chan_t1_rvalid, %chan_t1_rready) : (i1, i1, !hw.inout<i8>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i8>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t2_fifo" @mygo.fifo_i1_d1(%clk, %rst, %chan_t2_wdata, %chan_t2_wvalid, %chan_t2_wready, %chan_t2_rdata, %chan_t2_rvalid, %chan_t2_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "main_inst0" @main__proc_main(%clk, %rst, %chan_t2_rdata, %chan_t2_rvalid, %chan_t2_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "stage1_inst1" @main__proc_stage1(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "stage2_inst2" @main__proc_stage2(%clk, %rst, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "stage3_inst3" @main__proc_stage3(%clk, %rst, %chan_t2_wdata, %chan_t2_wvalid, %chan_t2_wready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.output
  }
  hw.module @main__proc_main(%clk: i1, %rst: i1, %chan_t2_rdata: !hw.inout<i1>, %chan_t2_rvalid: !hw.inout<i1>, %chan_t2_rready: !hw.inout<i1>) {
    // spawn stage3 stage=1 parent_stage=0
    // spawn stage2 stage=2 parent_stage=0
    // spawn stage1 stage=3 parent_stage=0
    %v0 = sv.read_inout %chan_t2_rdata : !hw.inout<i1>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t2_rready, %c_bool_0 : !hw.inout<i1>, i1
  }
  hw.module @main__proc_stage1(%clk: i1, %rst: i1, %chan_t0_wdata: !hw.inout<i32>, %chan_t0_wvalid: !hw.inout<i1>, %chan_t0_wready: !hw.inout<i1>) {
    %c0 = hw.constant 4 : i32
    %c1 = hw.constant 0 : i32
    %c2 = hw.constant 4 : i32
    %c3 = hw.constant 1 : i32
    sv.assign %chan_t0_wdata, %c0 : !hw.inout<i32>, i32
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    // phi t0_16 has 2 incoming values
    %v4 = comb.icmp ult, %t0_16, %c2 : i32
    %v5 = comb.add %t0_16, %t0_16 : i32
    sv.assign %chan_t0_wdata, %v5 : !hw.inout<i32>, i32
    sv.assign %chan_t0_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v6 = comb.add %t0_16, %c3 : i32
  }
  hw.module @main__proc_stage2(%clk: i1, %rst: i1, %chan_t0_rdata: !hw.inout<i32>, %chan_t0_rvalid: !hw.inout<i1>, %chan_t0_rready: !hw.inout<i1>) {
    %c0 = hw.constant 4 : i32
    %c1 = hw.constant 1 : i32
    %c2 = hw.constant 0 : i32
    %v3 = sv.read_inout %chan_t0_rdata : !hw.inout<i32>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_rready, %c_bool_0 : !hw.inout<i1>, i1
    // phi t2_8 has 2 incoming values
    %v4 = comb.icmp ult, %t2_8, %c0 : i32
    %v5 = sv.read_inout %chan_t0_rdata : !hw.inout<i32>
    sv.assign %chan_t0_rready, %c_bool_0 : !hw.inout<i1>, i1
    %v6 = comb.add %t2_8, %c1 : i32
  }
  hw.module @main__proc_stage3(%clk: i1, %rst: i1, %chan_t2_wdata: !hw.inout<i1>, %chan_t2_wvalid: !hw.inout<i1>, %chan_t2_wready: !hw.inout<i1>) {
    %c0 = hw.constant 0 : i32
    %c1 = hw.constant 4 : i32
    %c2 = hw.constant 1 : i32
    %c3 = hw.constant true : i1
    // phi t1_0 has 2 incoming values
    %v4 = comb.icmp ult, %t1_0, %c1 : i32
    sv.assign %chan_t2_wdata, %c3 : !hw.inout<i1>, i1
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t2_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v5 = comb.add %t1_0, %c2 : i32
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
  hw.module @mygo.fifo_i8_d8(%clk: i1, %rst: i1, %in_data: !hw.inout<i8>, %in_valid: !hw.inout<i1>, %in_ready: !hw.inout<i1>, %out_data: !hw.inout<i8>, %out_valid: !hw.inout<i1>, %out_ready: !hw.inout<i1>) {
    // simple passthrough FIFO depth=8
    %write_data = sv.read_inout %in_data : !hw.inout<i8>
    %write_valid = sv.read_inout %in_valid : !hw.inout<i1>
    sv.assign %out_data, %write_data : !hw.inout<i8>, i8
    sv.assign %out_valid, %write_valid : !hw.inout<i1>, i1
    %const_ready = hw.constant true : i1
    sv.assign %in_ready, %const_ready : !hw.inout<i1>, i1
  }
}
