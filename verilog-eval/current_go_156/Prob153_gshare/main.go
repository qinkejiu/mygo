package main

const phtInit153 uint64 = 0x5555555555555555

var pht0_153 uint64 = phtInit153
var pht1_153 uint64 = phtInit153
var pht2_153 uint64 = phtInit153
var pht3_153 uint64 = phtInit153
var history_153 uint8
var predict_taken_pre_153 bool
var predict_history_shift_153 uint8
var out_predict_taken bool
var out_predict_history uint8

func TopModule(
	clk bool,
	areset bool,
	predict_valid bool,
	predict_pc uint8,
	train_valid bool,
	train_taken bool,
	train_mispredicted bool,
	train_history uint8,
	train_pc uint8,
) {
	predictIndexPre := (history_153 ^ predict_pc) & 0x7f
	predictShiftPre := (predictIndexPre & 0x1f) << 1
	var predictCounterPre uint8
	switch (predictIndexPre >> 5) & 0x03 {
	case 0:
		predictCounterPre = uint8((pht0_153 >> predictShiftPre) & 0x03)
	case 1:
		predictCounterPre = uint8((pht1_153 >> predictShiftPre) & 0x03)
	case 2:
		predictCounterPre = uint8((pht2_153 >> predictShiftPre) & 0x03)
	default:
		predictCounterPre = uint8((pht3_153 >> predictShiftPre) & 0x03)
	}
	predict_taken_pre_153 = (predictCounterPre & 0x02) != 0
	predict_history_shift_153 = (history_153 << 1) & 0x7f
	if predict_taken_pre_153 {
		predict_history_shift_153 |= 0x01
	}

	if areset {
		pht0_153 = phtInit153
		pht1_153 = phtInit153
		pht2_153 = phtInit153
		pht3_153 = phtInit153
		history_153 = 0
	} else if clk {
		if train_valid && train_mispredicted {
			history_153 = (train_history << 1) & 0x7f
			if train_taken {
				history_153 |= 0x01
			}
		} else if predict_valid {
			history_153 = predict_history_shift_153 & 0x7f
		}

		if train_valid {
			trainIndex := (train_history ^ train_pc) & 0x7f
			trainShift := (trainIndex & 0x1f) << 1
			var counter uint8
			switch (trainIndex >> 5) & 0x03 {
			case 0:
				counter = uint8((pht0_153 >> trainShift) & 0x03)
			case 1:
				counter = uint8((pht1_153 >> trainShift) & 0x03)
			case 2:
				counter = uint8((pht2_153 >> trainShift) & 0x03)
			default:
				counter = uint8((pht3_153 >> trainShift) & 0x03)
			}
			if train_taken {
				if counter < 3 {
					counter++
				}
			} else if counter > 0 {
				counter--
			}

			mask := ^(uint64(0x03) << trainShift)
			bits := uint64(counter&0x03) << trainShift
			switch (trainIndex >> 5) & 0x03 {
			case 0:
				pht0_153 = (pht0_153 & mask) | bits
			case 1:
				pht1_153 = (pht1_153 & mask) | bits
			case 2:
				pht2_153 = (pht2_153 & mask) | bits
			default:
				pht3_153 = (pht3_153 & mask) | bits
			}
		}
	}

	if predict_valid {
		predictIndex := (history_153 ^ predict_pc) & 0x7f
		predictShift := (predictIndex & 0x1f) << 1
		var predictCounter uint8
		switch (predictIndex >> 5) & 0x03 {
		case 0:
			predictCounter = uint8((pht0_153 >> predictShift) & 0x03)
		case 1:
			predictCounter = uint8((pht1_153 >> predictShift) & 0x03)
		case 2:
			predictCounter = uint8((pht2_153 >> predictShift) & 0x03)
		default:
			predictCounter = uint8((pht3_153 >> predictShift) & 0x03)
		}
		out_predict_taken = (predictCounter & 0x02) != 0
		out_predict_history = history_153 & 0x7f
	} else {
		out_predict_taken = false
		out_predict_history = 0
	}
}

func main() {}
