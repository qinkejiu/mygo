package main

var out_pm bool
var out_hh uint8
var out_mm uint8
var out_ss uint8

func TopModule(clk bool, reset bool, ena bool) {
    // Sequential logic triggered on positive edge of clk
    // We'll model this with a simple conditional check
    // In real hardware, this would be clocked logic
    
    // Internal state registers
    var pm_reg bool
    var hh_reg uint8
    var mm_reg uint8
    var ss_reg uint8
    
    // Reset has highest priority
    if reset {
        // Reset to "12:00 AM"
        // 12 in BCD: tens=1, units=2 -> 0x12
        // 00 in BCD: tens=0, units=0 -> 0x00
        // AM: pm=0
        pm_reg = false
        hh_reg = 0x12  // BCD: 0001 0010
        mm_reg = 0x00  // BCD: 0000 0000
        ss_reg = 0x00  // BCD: 0000 0000
    } else if ena {
        // Extract BCD digits
        ss_units := ss_reg & 0x0F
        ss_tens := (ss_reg >> 4) & 0x0F
        mm_units := mm_reg & 0x0F
        mm_tens := (mm_reg >> 4) & 0x0F
        hh_units := hh_reg & 0x0F
        hh_tens := (hh_reg >> 4) & 0x0F
        
        // Determine which counters should increment
        // enable[0]: always true (seconds units)
        // enable[1]: ss_units == 9
        // enable[2]: ss_tens == 5 && ss_units == 9
        // enable[3]: mm_units == 9 && ss_tens == 5 && ss_units == 9
        // enable[4]: mm_tens == 5 && mm_units == 9 && ss_tens == 5 && ss_units == 9
        // enable[5]: hh_units == 9 && mm_tens == 5 && mm_units == 9 && ss_tens == 5 && ss_units == 9
        // enable[6]: hh_reg == 0x12 && hh_units == 9 && mm_tens == 5 && mm_units == 9 && ss_tens == 5 && ss_units == 9
        
        // Increment seconds units
        if ss_units == 9 {
            ss_units = 0
        } else {
            ss_units++
        }
        
        // Increment seconds tens
        if ss_units == 0 && ss_tens == 5 {
            ss_tens = 0
        } else if ss_units == 0 {
            ss_tens++
        }
        
        // Increment minutes units
        if ss_units == 0 && ss_tens == 0 && mm_units == 9 {
            mm_units = 0
        } else if ss_units == 0 && ss_tens == 0 {
            mm_units++
        }
        
        // Increment minutes tens
        if ss_units == 0 && ss_tens == 0 && mm_units == 0 && mm_tens == 5 {
            mm_tens = 0
        } else if ss_units == 0 && ss_tens == 0 && mm_units == 0 {
            mm_tens++
        }
        
        // Increment hours units
        if ss_units == 0 && ss_tens == 0 && mm_units == 0 && mm_tens == 0 && hh_units == 9 {
            hh_units = 0
        } else if ss_units == 0 && ss_tens == 0 && mm_units == 0 && mm_tens == 0 {
            // Special handling for 12-hour clock
            if hh_reg == 0x12 {
                hh_units = 1
            } else {
                hh_units++
            }
        }
        
        // Increment hours tens
        if ss_units == 0 && ss_tens == 0 && mm_units == 0 && mm_tens == 0 && hh_units == 0 {
            if hh_tens == 1 && hh_units == 0 {
                // Going from 09 to 10
                hh_tens = 1
            } else if hh_reg == 0x09 {
                // 09 -> 10
                hh_tens = 1
            } else if hh_reg == 0x12 {
                // 12 -> 01 (tens becomes 0)
                hh_tens = 0
            }
        }
        
        // Toggle PM/AM when reaching 12:59:59
        if ss_units == 0 && ss_tens == 0 && mm_units == 0 && mm_tens == 0 && 
           hh_units == 0 && hh_tens == 0 && hh_reg == 0x12 {
            pm_reg = !pm_reg
        }
        
        // Reconstruct BCD values
        ss_reg = (ss_tens << 4) | ss_units
        mm_reg = (mm_tens << 4) | mm_units
        hh_reg = (hh_tens << 4) | hh_units
        
        // Handle special case: after 12:59:59, go to 01:00:00
        if ss_units == 0 && ss_tens == 0 && mm_units == 0 && mm_tens == 0 && 
           hh_units == 0 && hh_tens == 0 && hh_reg == 0x00 {
            hh_reg = 0x01
        }
    }
    
    // Assign outputs
    out_pm = pm_reg
    out_hh = hh_reg
    out_mm = mm_reg
    out_ss = ss_reg
}

func main() {}
