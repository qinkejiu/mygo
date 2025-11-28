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
    //   producer producer stage 5
    //   consumer router stage 3
    // channel t1 depth=1 type=i32
    %chan_t1_wdata = sv.wire : !hw.inout<i32>
    %chan_t1_wvalid = sv.wire : !hw.inout<i1>
    %chan_t1_wready = sv.wire : !hw.inout<i1>
    %chan_t1_rdata = sv.wire : !hw.inout<i32>
    %chan_t1_rvalid = sv.wire : !hw.inout<i1>
    %chan_t1_rready = sv.wire : !hw.inout<i1>
    // channel t1 occupancy 0/1
    //   consumer router stage 3
    // channel t2 depth=1 type=i32
    %chan_t2_wdata = sv.wire : !hw.inout<i32>
    %chan_t2_wvalid = sv.wire : !hw.inout<i1>
    %chan_t2_wready = sv.wire : !hw.inout<i1>
    %chan_t2_rdata = sv.wire : !hw.inout<i32>
    %chan_t2_rvalid = sv.wire : !hw.inout<i1>
    %chan_t2_rready = sv.wire : !hw.inout<i1>
    // channel t2 occupancy 0/1
    //   consumer consumer stage 2
    // channel t3 depth=1 type=i32
    %chan_t3_wdata = sv.wire : !hw.inout<i32>
    %chan_t3_wvalid = sv.wire : !hw.inout<i1>
    %chan_t3_wready = sv.wire : !hw.inout<i1>
    %chan_t3_rdata = sv.wire : !hw.inout<i32>
    %chan_t3_rvalid = sv.wire : !hw.inout<i1>
    %chan_t3_rready = sv.wire : !hw.inout<i1>
    // channel t3 occupancy 0/1
    // channel t4 depth=1 type=i1
    %chan_t4_wdata = sv.wire : !hw.inout<i1>
    %chan_t4_wvalid = sv.wire : !hw.inout<i1>
    %chan_t4_wready = sv.wire : !hw.inout<i1>
    %chan_t4_rdata = sv.wire : !hw.inout<i1>
    %chan_t4_rvalid = sv.wire : !hw.inout<i1>
    %chan_t4_rready = sv.wire : !hw.inout<i1>
    // channel t4 occupancy 0/1
    //   producer router stage 3
    //   consumer producer stage 5
    // channel t5 depth=1 type=i1
    %chan_t5_wdata = sv.wire : !hw.inout<i1>
    %chan_t5_wvalid = sv.wire : !hw.inout<i1>
    %chan_t5_wready = sv.wire : !hw.inout<i1>
    %chan_t5_rdata = sv.wire : !hw.inout<i1>
    %chan_t5_rvalid = sv.wire : !hw.inout<i1>
    %chan_t5_rready = sv.wire : !hw.inout<i1>
    // channel t5 occupancy 1/1
    //   producer router stage 3
    // channel t6 depth=1 type=i1
    %chan_t6_wdata = sv.wire : !hw.inout<i1>
    %chan_t6_wvalid = sv.wire : !hw.inout<i1>
    %chan_t6_wready = sv.wire : !hw.inout<i1>
    %chan_t6_rdata = sv.wire : !hw.inout<i1>
    %chan_t6_rvalid = sv.wire : !hw.inout<i1>
    %chan_t6_rready = sv.wire : !hw.inout<i1>
    // channel t6 occupancy 1/1
    //   producer consumer stage 2
    // channel t7 depth=1 type=i1
    %chan_t7_wdata = sv.wire : !hw.inout<i1>
    %chan_t7_wvalid = sv.wire : !hw.inout<i1>
    %chan_t7_wready = sv.wire : !hw.inout<i1>
    %chan_t7_rdata = sv.wire : !hw.inout<i1>
    %chan_t7_rvalid = sv.wire : !hw.inout<i1>
    %chan_t7_rready = sv.wire : !hw.inout<i1>
    // channel t7 occupancy 0/1
    // channel t8 depth=2 type=i1
    %chan_t8_wdata = sv.wire : !hw.inout<i1>
    %chan_t8_wvalid = sv.wire : !hw.inout<i1>
    %chan_t8_wready = sv.wire : !hw.inout<i1>
    %chan_t8_rdata = sv.wire : !hw.inout<i1>
    %chan_t8_rvalid = sv.wire : !hw.inout<i1>
    %chan_t8_rready = sv.wire : !hw.inout<i1>
    // channel t8 occupancy 0/2
    //   producer consumer stage 2
    //   consumer main stage 0
    hw.instance "t0_fifo" @mygo.fifo_i32_d1(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t1_fifo" @mygo.fifo_i32_d1(%clk, %rst, %chan_t1_wdata, %chan_t1_wvalid, %chan_t1_wready, %chan_t1_rdata, %chan_t1_rvalid, %chan_t1_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t2_fifo" @mygo.fifo_i32_d1(%clk, %rst, %chan_t2_wdata, %chan_t2_wvalid, %chan_t2_wready, %chan_t2_rdata, %chan_t2_rvalid, %chan_t2_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t3_fifo" @mygo.fifo_i32_d1(%clk, %rst, %chan_t3_wdata, %chan_t3_wvalid, %chan_t3_wready, %chan_t3_rdata, %chan_t3_rvalid, %chan_t3_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t4_fifo" @mygo.fifo_i1_d1(%clk, %rst, %chan_t4_wdata, %chan_t4_wvalid, %chan_t4_wready, %chan_t4_rdata, %chan_t4_rvalid, %chan_t4_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t5_fifo" @mygo.fifo_i1_d1(%clk, %rst, %chan_t5_wdata, %chan_t5_wvalid, %chan_t5_wready, %chan_t5_rdata, %chan_t5_rvalid, %chan_t5_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t6_fifo" @mygo.fifo_i1_d1(%clk, %rst, %chan_t6_wdata, %chan_t6_wvalid, %chan_t6_wready, %chan_t6_rdata, %chan_t6_rvalid, %chan_t6_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t7_fifo" @mygo.fifo_i1_d1(%clk, %rst, %chan_t7_wdata, %chan_t7_wvalid, %chan_t7_wready, %chan_t7_rdata, %chan_t7_rvalid, %chan_t7_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t8_fifo" @mygo.fifo_i1_d2(%clk, %rst, %chan_t8_wdata, %chan_t8_wvalid, %chan_t8_wready, %chan_t8_rdata, %chan_t8_rvalid, %chan_t8_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "consumer_inst0" @main__proc_consumer(%clk, %rst, %chan_t2_rdata, %chan_t2_rvalid, %chan_t2_rready, %chan_t6_wdata, %chan_t6_wvalid, %chan_t6_wready, %chan_t8_wdata, %chan_t8_wvalid, %chan_t8_wready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "main_inst1" @main__proc_main(%clk, %rst, %chan_t8_rdata, %chan_t8_rvalid, %chan_t8_rready) : (i1, i1, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "producer_inst2" @main__proc_producer(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready, %chan_t4_rdata, %chan_t4_rvalid, %chan_t4_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "router_inst3" @main__proc_router(%clk, %rst, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready, %chan_t1_rdata, %chan_t1_rvalid, %chan_t1_rready, %chan_t4_wdata, %chan_t4_wvalid, %chan_t4_wready, %chan_t5_wdata, %chan_t5_wvalid, %chan_t5_wready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.output
  }
  hw.module @main__proc_consumer(%clk: i1, %rst: i1, %chan_t2_rdata: !hw.inout<i32>, %chan_t2_rvalid: !hw.inout<i1>, %chan_t2_rready: !hw.inout<i1>, %chan_t6_wdata: !hw.inout<i1>, %chan_t6_wvalid: !hw.inout<i1>, %chan_t6_wready: !hw.inout<i1>, %chan_t8_wdata: !hw.inout<i1>, %chan_t8_wvalid: !hw.inout<i1>, %chan_t8_wready: !hw.inout<i1>) {
    %c0 = hw.constant 0 : i32
    %c1 = hw.constant 4 : i32
    %c2 = hw.constant true : i1
    %c3 = hw.constant 1 : i32
    %c4 = hw.constant true : i1
    // phi t0_1 has 2 incoming values
    %v5 = comb.icmp slt, %t0_1, %c1 : i32
    sv.assign %chan_t8_wdata, %c4 : !hw.inout<i1>, i1
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t8_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v6 = sv.read_inout %chan_t2_rdata : !hw.inout<i32>
    sv.assign %chan_t2_rready, %c_bool_0 : !hw.inout<i1>, i1
    %v7 = seq.compreg %v6, %clk : i32
    sv.assign %chan_t6_wdata, %c2 : !hw.inout<i1>, i1
    sv.assign %chan_t6_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v8 = comb.add %t0_1, %c3 : i32
  }
  hw.module @main__proc_main(%clk: i1, %rst: i1, %chan_t8_rdata: !hw.inout<i1>, %chan_t8_rvalid: !hw.inout<i1>, %chan_t8_rready: !hw.inout<i1>) {
    %c0 = hw.constant 0 : i32
    %c1 = hw.constant 1 : i32
    %c2 = hw.constant 0 : i32
    %c3 = hw.constant 0 : i32
    %c4 = hw.constant 1 : i32
    %c5 = hw.constant 1 : i32
    %c6 = hw.constant 0 : i32
    %c7 = hw.constant 2 : i32
    %c8 = hw.constant 1 : i32
    // spawn consumer stage=2 parent_stage=0
    // spawn consumer stage=2 parent_stage=0
    // spawn router stage=3 parent_stage=0
    // spawn producer stage=5 parent_stage=0
    // spawn producer stage=5 parent_stage=0
    // phi t27_38 has 2 incoming values
    %v9 = comb.icmp slt, %t27_38, %c7 : i32
    %v10 = sv.read_inout %chan_t8_rdata : !hw.inout<i1>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t8_rready, %c_bool_0 : !hw.inout<i1>, i1
    %v11 = comb.add %t27_38, %c8 : i32
  }
  hw.module @main__proc_producer(%clk: i1, %rst: i1, %chan_t0_wdata: !hw.inout<i32>, %chan_t0_wvalid: !hw.inout<i1>, %chan_t0_wready: !hw.inout<i1>, %chan_t4_rdata: !hw.inout<i1>, %chan_t4_rvalid: !hw.inout<i1>, %chan_t4_rready: !hw.inout<i1>) {
    %c0 = hw.constant 0 : i32
    %c1 = hw.constant 0 : i32
    %c2 = hw.constant 0 : i32
    %c3 = hw.constant 4 : i32
    %c4 = hw.constant 1 : i32
    %c5 = hw.constant 10 : i32
    %c6 = hw.constant 1 : i32
    // phi t0_23 has 2 incoming values
    %v7 = comb.icmp slt, %t0_23, %c3 : i32
    %v8 = comb.add %c1, %t0_23 : i32
    %v9 = comb.and %v8, %c4 : i32
    %v10 = comb.mul %c0, %c5 : i32
    %v11 = comb.add %v10, %t0_23 : i32
    sv.assign %chan_t0_wdata, %complit : !hw.inout<i32>, i32
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v12 = sv.read_inout %chan_t4_rdata : !hw.inout<i1>
    sv.assign %chan_t4_rready, %c_bool_0 : !hw.inout<i1>, i1
    %v13 = comb.add %t0_23, %c6 : i32
  }
  hw.module @main__proc_router(%clk: i1, %rst: i1, %chan_t0_rdata: !hw.inout<i32>, %chan_t0_rvalid: !hw.inout<i1>, %chan_t0_rready: !hw.inout<i1>, %chan_t1_rdata: !hw.inout<i32>, %chan_t1_rvalid: !hw.inout<i1>, %chan_t1_rready: !hw.inout<i1>, %chan_t4_wdata: !hw.inout<i1>, %chan_t4_wvalid: !hw.inout<i1>, %chan_t4_wready: !hw.inout<i1>, %chan_t5_wdata: !hw.inout<i1>, %chan_t5_wvalid: !hw.inout<i1>, %chan_t5_wready: !hw.inout<i1>) {
    %c0 = hw.constant 0 : i32
    %c1 = hw.constant 4 : i32
    %c2 = hw.constant true : i1
    %c3 = hw.constant true : i1
    %c4 = hw.constant 1 : i32
    // phi t0_11 has 2 incoming values
    %v5 = comb.icmp slt, %t0_11, %c1 : i32
    %v6 = sv.read_inout %chan_t0_rdata : !hw.inout<i32>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_rready, %c_bool_0 : !hw.inout<i1>, i1
    sv.assign %chan_t4_wdata, %c2 : !hw.inout<i1>, i1
    sv.assign %chan_t4_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v7 = sv.read_inout %chan_t1_rdata : !hw.inout<i32>
    sv.assign %chan_t1_rready, %c_bool_0 : !hw.inout<i1>, i1
    sv.assign %chan_t5_wdata, %c3 : !hw.inout<i1>, i1
    sv.assign %chan_t5_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v8 = comb.add %t0_11, %c4 : i32
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
  hw.module @mygo.fifo_i1_d2(%clk: i1, %rst: i1, %in_data: !hw.inout<i1>, %in_valid: !hw.inout<i1>, %in_ready: !hw.inout<i1>, %out_data: !hw.inout<i1>, %out_valid: !hw.inout<i1>, %out_ready: !hw.inout<i1>) {
    // simple passthrough FIFO depth=2
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
}
