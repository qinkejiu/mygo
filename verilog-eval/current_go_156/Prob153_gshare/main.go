package main

const phtInit153 uint64 = 0x5555555555555555

var pht0_153 uint64 = phtInit153
var pht1_153 uint64 = phtInit153
var pht2_153 uint64 = phtInit153
var pht3_153 uint64 = phtInit153
var history_153 uint8
var out_predict_taken bool
var out_predict_history uint8

func phtGet153(index uint8) uint8 {
	shift := (index & 0x1f) << 1
	switch (index >> 5) & 0x03 {
	case 0:
		return uint8((pht0_153 >> shift) & 0x03)
	case 1:
		return uint8((pht1_153 >> shift) & 0x03)
	case 2:
		return uint8((pht2_153 >> shift) & 0x03)
	default:
		return uint8((pht3_153 >> shift) & 0x03)
	}
}

func phtSet153(index uint8, value uint8) {
	shift := (index & 0x1f) << 1
	mask := ^(uint64(0x03) << shift)
	bits := uint64(value&0x03) << shift
	switch (index >> 5) & 0x03 {
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
	if areset {
		pht0_153 = phtInit153
		pht1_153 = phtInit153
		pht2_153 = phtInit153
		pht3_153 = phtInit153
		history_153 = 0
	} else if clk {
		predictIndex := (history_153 ^ predict_pc) & 0x7f
		predictCounter := phtGet153(predictIndex)
		predictTaken := (predictCounter & 0x02) != 0
		nextHistory := history_153

		if predict_valid {
			nextHistory = (history_153 << 1) & 0x7f
			if predictTaken {
				nextHistory |= 0x01
			}
		}

		if train_valid {
			trainIndex := (train_history ^ train_pc) & 0x7f
			counter := phtGet153(trainIndex)

			if train_taken {
				if counter < 3 {
					counter++
				}
			} else if counter > 0 {
				counter--
			}
			phtSet153(trainIndex, counter)

			if train_mispredicted {
				nextHistory = (train_history << 1) & 0x7f
				if train_taken {
					nextHistory |= 0x01
				}
			}
		}

		history_153 = nextHistory & 0x7f
	}

	if predict_valid {
		predictIndex := (history_153 ^ predict_pc) & 0x7f
		out_predict_taken = (phtGet153(predictIndex) & 0x02) != 0
		out_predict_history = history_153 & 0x7f
	} else {
		out_predict_taken = false
		out_predict_history = 0
	}
}

func main() {}
