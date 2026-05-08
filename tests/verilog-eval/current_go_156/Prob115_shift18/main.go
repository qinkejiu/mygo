package main

const signMask115 uint64 = 0x8000000000000000

var prev_clk_115 bool
var q_reg_115 uint64
var out_q uint64

func TopModule(clk bool, load bool, ena bool, amount uint8, data uint64) {
	if clk && !prev_clk_115 {
		old := q_reg_115
		if load {
			q_reg_115 = data
		} else if ena {
			switch amount & 0x03 {
			case 0x00:
				q_reg_115 = old << 1
			case 0x01:
				q_reg_115 = old << 8
			case 0x02:
				q_reg_115 = (old >> 1) | (old & signMask115)
			case 0x03:
				q_reg_115 = old >> 8
				if (old & signMask115) != 0 {
					q_reg_115 |= 0xFF00000000000000
				}
			}
		}
	}

	prev_clk_115 = clk
	out_q = q_reg_115
}

func main() {}
