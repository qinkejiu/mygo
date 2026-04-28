package main

var out_pm bool
var out_hh uint8
var out_mm uint8
var out_ss uint8

func TopModule(clk bool, reset bool, ena bool) {
    // Internal registers
    var pm_reg bool
    var hh_reg uint8
    var mm_reg uint8
    var ss_reg uint8

    // Enable signals for each digit
    var enable [7]bool

    // Calculate enable signals
    // enable[0]: always 1
    enable[0] = true

    // enable[1]: ss[3:0] == 9
    enable[1] = (ss_reg & 0x0F) == 0x09

    // enable[2]: ss[7:0] == 0x59
    enable[2] = ss_reg == 0x59

    // enable[3]: mm[3:0] == 9 and ss[7:0] == 0x59
    enable[3] = ((mm_reg & 0x0F) == 0x09) && (ss_reg == 0x59)

    // enable[4]: mm[7:0] == 0x59 and ss[7:0] == 0x59
    enable[4] = (mm_reg == 0x59) && (ss_reg == 0x59)

    // enable[5]: hh[3:0] == 9 and mm[7:0] == 0x59 and ss[7:0] == 0x59
    enable[5] = ((hh_reg & 0x0F) == 0x09) && (mm_reg == 0x59) && (ss_reg == 0x59)

    // enable[6]: hh[7:0] == 0x12 and mm[7:0] == 0x59 and ss[7:0] == 0x59
    enable[6] = (hh_reg == 0x12) && (mm_reg == 0x59) && (ss_reg == 0x59)

    // Clocked logic
    if clk {
        if reset {
            // Reset to 12:00:00 AM
            pm_reg = false
            hh_reg = 0x12
            mm_reg = 0x00
            ss_reg = 0x00
        } else if ena {
            // Seconds: ones digit
            if enable[0] && ((ss_reg & 0x0F) == 0x09) {
                ss_reg = (ss_reg & 0xF0) | 0x00
            } else if enable[0] {
                ss_reg = (ss_reg & 0xF0) | ((ss_reg & 0x0F) + 1)
            }

            // Seconds: tens digit
            if enable[1] && ((ss_reg & 0xF0) == 0x50) {
                ss_reg = (ss_reg & 0x0F) | 0x00
            } else if enable[1] {
                ss_reg = (ss_reg & 0x0F) | (((ss_reg & 0xF0) + 0x10) & 0xF0)
            }

            // Minutes: ones digit
            if enable[2] && ((mm_reg & 0x0F) == 0x09) {
                mm_reg = (mm_reg & 0xF0) | 0x00
            } else if enable[2] {
                mm_reg = (mm_reg & 0xF0) | ((mm_reg & 0x0F) + 1)
            }

            // Minutes: tens digit
            if enable[3] && ((mm_reg & 0xF0) == 0x50) {
                mm_reg = (mm_reg & 0x0F) | 0x00
            } else if enable[3] {
                mm_reg = (mm_reg & 0x0F) | (((mm_reg & 0xF0) + 0x10) & 0xF0)
            }

            // Hours: ones digit
            if enable[4] && ((hh_reg & 0x0F) == 0x09) {
                hh_reg = (hh_reg & 0xF0) | 0x00
            } else if enable[4] {
                hh_reg = (hh_reg & 0xF0) | ((hh_reg & 0x0F) + 1)
            }

            // Hours: tens digit and rollover from 12 to 01
            if enable[4] && (hh_reg == 0x12) {
                hh_reg = 0x01
            } else if enable[5] {
                hh_reg = (hh_reg & 0x0F) | (((hh_reg & 0xF0) + 0x10) & 0xF0)
            }

            // Toggle PM/AM
            if enable[6] {
                pm_reg = !pm_reg
            }
        }
    }

    // Assign outputs
    out_pm = pm_reg
    out_hh = hh_reg
    out_mm = mm_reg
    out_ss = ss_reg
}

func main() {}
