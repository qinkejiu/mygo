# Current Failure Classification

- Total cases: 156
- Equivalent: 66
- Not equivalent: 88
- Iverilog compile failed: 2

## Iverilog Compile Failed

- `Prob099_m2014_q6c`
  /home/qinkejiu/mygo/verilog-eval/runs/basic_fix_batch_input/tests/Prob099_m2014_q6c_test.sv:71: error: port ``Y2'' is not a port of good1.
  /home/qinkejiu/mygo/verilog-eval/runs/basic_fix_batch_input/tests/Prob099_m2014_q6c_test.sv:71: error: port ``Y4'' is not a port of good1.
  /home/qinkejiu/mygo/verilog-eval/runs/basic_fix_batch_input/tests/Prob099_m2014_q6c_test.sv:77: error: port ``Y2'' is not a port of top_module1.
  /home/qinkejiu/mygo/verilog-eval/runs/basic_fix_batch_input/tests/Prob099_m2014_q6c_test.sv:77: error: port ``Y4'' is not a port of top_module1.
- `Prob148_2013_q2afsm`
  /home/qinkejiu/mygo/verilog-eval/runs/basic_fix_batch_input/tests/Prob148_2013_q2afsm_test.sv:145: error: port ``g'' is not a port of top_module1.
  /home/qinkejiu/mygo/verilog-eval/runs/basic_fix_batch_input/tests/Prob148_2013_q2afsm_test.sv:145: warning: Port 3 (r) of TopModule expects 8 bits, got 3.
  /home/qinkejiu/mygo/verilog-eval/runs/basic_fix_batch_input/tests/Prob148_2013_q2afsm_test.sv:145:        : Padding 5 high bits of the port.
  1 error(s) during elaboration.

## Not Equivalent

These cases compile and elaborate, but simulation mismatches remain.

- `Prob017_mux2to1v`: mismatches `108/114`
- `Prob018_mux256to1`: mismatches `983/2000`
- `Prob021_mux256to1v`: mismatches `1889/2000`
- `Prob023_vector100r`: mismatches `199/200`
- `Prob027_fadd`: mismatches `101/214`
- `Prob028_m2014_q4a`: mismatches `19/100`
- `Prob030_popcount255`: mismatches `209/215`
- `Prob031_dff`: mismatches `118/121`
- `Prob034_dff8`: mismatches `38/41`
- `Prob035_count1to10`: mismatches `384/439`
- `Prob037_review2015_count1k`: mismatches `7994/8027`
- `Prob038_count15`: mismatches `380/421`
- `Prob040_count10`: mismatches `384/439`
- `Prob043_vector5`: mismatches `99/100`
- `Prob045_edgedetect2`: mismatches `207/228`
- `Prob046_dff8p`: mismatches `418/436`
- `Prob052_gates100`: mismatches `423/433`
- `Prob054_edgedetect`: mismatches `186/227`
- `Prob055_conditional`: mismatches `38/112`
- `Prob056_ece241_2013_q7`: mismatches `189/422`
- `Prob057_kmap2`: mismatches `90/232`
- `Prob060_m2014_q4k`: mismatches `84/299`
- `Prob063_review2015_shiftcount`: mismatches `1906/2071`
- `Prob066_edgecapture`: mismatches `227/266`
- `Prob067_countslow`: mismatches `431/499`
- `Prob068_countbcd`: mismatches `39762/39805`
- `Prob069_truthtable1`: mismatches `8/58`
- `Prob070_ece241_2013_q2`: mismatches `49/107`
- `Prob073_dff16e`: mismatches `428/443`
- `Prob074_ece241_2014_q4`: mismatches `23/118`
- `Prob075_counter_2bc`: mismatches `771/1051`
- `Prob077_wire_decl`: mismatches `68/122`
- `Prob078_dualedge`: mismatches `111/224`
- `Prob079_fsm3onehot`: mismatches `106/200`
- `Prob080_timer`: mismatches `5073/7127`
- `Prob081_7458`: mismatches `415/439`
- `Prob082_lfsr32`: mismatches `199953/200000`
- `Prob084_ece241_2013_q12`: mismatches `239/530`
- `Prob085_shift4`: mismatches `96/427`
- `Prob086_lfsr5`: mismatches `4276/4443`
- `Prob088_ece241_2014_q5b`: mismatches `210/436`
- `Prob089_ece241_2014_q5a`: mismatches `216/436`
- `Prob092_gatesv100`: mismatches `201/201`
- `Prob093_ece241_2014_q3`: mismatches `27/60`
- `Prob094_gatesv`: mismatches `207/213`
- `Prob095_review2015_fsmshift`: mismatches `123/200`
- `Prob096_review2015_fsmseq`: mismatches `308/643`
- `Prob102_circuit3`: mismatches `72/121`
- `Prob103_circuit2`: mismatches `121/121`
- `Prob105_rotate100`: mismatches `4002/4005`
- `Prob107_fsm1s`: mismatches `125/230`
- `Prob108_rule90`: mismatches `7078/7121`
- `Prob109_fsm1`: mismatches `103/228`
- `Prob110_fsm2`: mismatches `95/241`
- `Prob111_fsm2s`: mismatches `112/241`
- `Prob115_shift18`: mismatches `3184/4041`
- `Prob117_circuit9`: mismatches `206/245`
- `Prob119_fsm3`: mismatches `29/230`
- `Prob120_fsm3s`: mismatches `32/230`
- `Prob121_2014_q3bfsm`: mismatches `511/1006`
- `Prob124_rule110`: mismatches `6240/6283`
- `Prob127_lemmings1`: mismatches `120/229`
- `Prob128_fsm_ps2`: mismatches `90/400`
- `Prob129_ece241_2013_q8`: mismatches `46/440`
- `Prob131_mt2015_q4`: mismatches `60/200`
- `Prob133_2014_q3fsm`: mismatches `174/1414`
- `Prob134_2014_q3c`: mismatches `46/200`
- `Prob135_m2014_q6b`: mismatches `45/100`
- `Prob136_m2014_q6`: mismatches `136/200`
- `Prob137_fsm_serial`: mismatches `38/905`
- `Prob138_2012_q2fsm`: mismatches `279/400`
- `Prob139_2013_q2bfsm`: mismatches `487/1002`
- `Prob140_fsm_hdlc`: mismatches `224/801`
- `Prob141_count_clock`: mismatches `199984/200000`
- `Prob142_lemmings2`: mismatches `441/441`
- `Prob143_fsm_onehot`: mismatches `224/224`
- `Prob144_conwaylife`: mismatches `794/5023`
- `Prob145_circuit8`: mismatches `172/240`
- `Prob146_fsm_serialdata`: mismatches `38/905`
- `Prob147_circuit10`: mismatches `17/232`
- `Prob149_ece241_2013_q4`: mismatches `1403/2040`
- `Prob150_review2015_fsmonehot`: mismatches `300/300`
- `Prob151_review2015_fsm`: mismatches `4810/5069`
- `Prob152_lemmings3`: mismatches `443/443`
- `Prob153_gshare`: mismatches `461/1083`
- `Prob154_fsm_ps2data`: mismatches `490/1619`
- `Prob155_lemmings4`: mismatches `1003/1003`
- `Prob156_review2015_fancytimer`: mismatches `199075/200000`
