package main

var pm_141 bool
var hour_141 uint8 = 12
var min_tens_141 uint8
var min_ones_141 uint8
var sec_tens_141 uint8
var sec_ones_141 uint8
var out_pm bool
var out_hh uint8
var out_mm uint8
var out_ss uint8

func TopModule(clk bool, reset bool, ena bool) {
	if clk {
		if reset {
			pm_141 = false
			hour_141 = 12
			min_tens_141 = 0
			min_ones_141 = 0
			sec_tens_141 = 0
			sec_ones_141 = 0
		} else if ena {
			if sec_ones_141 < 9 {
				sec_ones_141++
			} else {
				sec_ones_141 = 0
				if sec_tens_141 < 5 {
					sec_tens_141++
				} else {
					sec_tens_141 = 0
					if min_ones_141 < 9 {
						min_ones_141++
					} else {
						min_ones_141 = 0
						if min_tens_141 < 5 {
							min_tens_141++
						} else {
							min_tens_141 = 0
							if hour_141 == 11 {
								hour_141 = 12
								pm_141 = !pm_141
							} else if hour_141 == 12 {
								hour_141 = 1
							} else {
								hour_141++
							}
						}
					}
				}
			}
		}
	}

	out_pm = pm_141
	out_hh = ((hour_141 / 10) << 4) | (hour_141 % 10)
	out_mm = (min_tens_141 << 4) | min_ones_141
	out_ss = (sec_tens_141 << 4) | sec_ones_141
}

func main() {}
