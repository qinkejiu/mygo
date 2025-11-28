module {
  hw.module @main(%clk: i1, %rst: i1) {
    // channel t0 depth=4 type=i32
    %chan_t0_wdata = sv.wire : !hw.inout<i32>
    %chan_t0_wvalid = sv.wire : !hw.inout<i1>
    %chan_t0_wready = sv.wire : !hw.inout<i1>
    %chan_t0_rdata = sv.wire : !hw.inout<i32>
    %chan_t0_rvalid = sv.wire : !hw.inout<i1>
    %chan_t0_rready = sv.wire : !hw.inout<i1>
    // channel t0 occupancy 1/4
    //   producer main stage 0
    //   consumer worker stage 1
    // channel t1 depth=4 type=i32
    %chan_t1_wdata = sv.wire : !hw.inout<i32>
    %chan_t1_wvalid = sv.wire : !hw.inout<i1>
    %chan_t1_wready = sv.wire : !hw.inout<i1>
    %chan_t1_rdata = sv.wire : !hw.inout<i32>
    %chan_t1_rvalid = sv.wire : !hw.inout<i1>
    %chan_t1_rready = sv.wire : !hw.inout<i1>
    // channel t1 occupancy 0/4
    //   producer worker stage 1
    //   consumer main stage 0
    hw.instance "t0_fifo" @mygo.fifo_i32_d4(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "t1_fifo" @mygo.fifo_i32_d4(%clk, %rst, %chan_t1_wdata, %chan_t1_wvalid, %chan_t1_wready, %chan_t1_rdata, %chan_t1_rvalid, %chan_t1_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "main_inst0" @main__proc_main(%clk, %rst, %chan_t0_wdata, %chan_t0_wvalid, %chan_t0_wready, %chan_t1_rdata, %chan_t1_rvalid, %chan_t1_rready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.instance "worker_inst1" @main__proc_worker(%clk, %rst, %chan_t0_rdata, %chan_t0_rvalid, %chan_t0_rready, %chan_t1_wdata, %chan_t1_wvalid, %chan_t1_wready) : (i1, i1, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>, !hw.inout<i32>, !hw.inout<i1>, !hw.inout<i1>) -> ()
    hw.output
  }
  hw.module @main__proc_main(%clk: i1, %rst: i1, %chan_t0_wdata: !hw.inout<i32>, %chan_t0_wvalid: !hw.inout<i1>, %chan_t0_wready: !hw.inout<i1>, %chan_t1_rdata: !hw.inout<i32>, %chan_t1_rvalid: !hw.inout<i1>, %chan_t1_rready: !hw.inout<i1>) {
    %c0 = hw.constant 5 : i32
    // spawn worker stage=1 parent_stage=0
    sv.assign %chan_t0_wdata, %c0 : !hw.inout<i32>, i32
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_wvalid, %c_bool_0 : !hw.inout<i1>, i1
    %v1 = sv.read_inout %chan_t1_rdata : !hw.inout<i32>
    sv.assign %chan_t1_rready, %c_bool_0 : !hw.inout<i1>, i1
  }
  hw.module @main__proc_worker(%clk: i1, %rst: i1, %chan_t0_rdata: !hw.inout<i32>, %chan_t0_rvalid: !hw.inout<i1>, %chan_t0_rready: !hw.inout<i1>, %chan_t1_wdata: !hw.inout<i32>, %chan_t1_wvalid: !hw.inout<i1>, %chan_t1_wready: !hw.inout<i1>) {
    %c0 = hw.constant 1 : i32
    %v1 = sv.read_inout %chan_t0_rdata : !hw.inout<i32>
    %c_bool_0 = hw.constant true : i1
    sv.assign %chan_t0_rready, %c_bool_0 : !hw.inout<i1>, i1
    %v2 = comb.add %v1, %c0 : i32
    sv.assign %chan_t1_wdata, %v2 : !hw.inout<i32>, i32
    sv.assign %chan_t1_wvalid, %c_bool_0 : !hw.inout<i1>, i1
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
