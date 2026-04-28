package main

var out_pm bool
var out_hh uint8
var out_mm uint8
var out_ss uint8

func TopModule(clk bool, reset bool, ena bool) {
    // Sequential logic triggered on positive edge of clk
    // We'll use a simple approach: track state in local variables
    // and update outputs on clock edges
    
    // State variables (registers)
    var pm_reg bool
    var hh_reg uint8
    var mm_reg uint8
    var ss_reg uint8
    
    // Clock edge detection (simplified - in real hardware this would be edge-triggered)
    // For modeling, we assume this function is called on each positive clock edge
    
    if reset {
        // Reset to 12:00:00 AM
        pm_reg = false
        hh_reg = 0x12  // BCD 12
        mm_reg = 0x00  // BCD 00
        ss_reg = 0x00  // BCD 00
    } else if ena {
        // Extract BCD digits
        ss_lo := ss_reg & 0x0F
        ss_hi := (ss_reg >> 4) & 0x0F
        mm_lo := mm_reg & 0x0F
        mm_hi := (mm_reg >> 4) & 0x0F
        hh_lo := hh_reg & 0x0F
        hh_hi := (hh_reg >> 4) & 0x0F
        
        // Seconds increment logic
        if ss_lo == 9 {
            ss_lo = 0
            if ss_hi == 5 {
                ss_hi = 0
                // Minutes increment logic
                if mm_lo == 9 {
                    mm_lo = 0
                    if mm_hi == 5 {
                        mm_hi = 0
                        // Hours increment logic
                        if hh_lo == 9 {
                            hh_lo = 0
                            hh_hi = 1
                        } else if hh_reg == 0x12 {
                            // Special case: 12 -> 01
                            hh_lo = 1
                            hh_hi = 0
                        } else if hh_reg == 0x09 {
                            // 09 -> 10
                            hh_lo = 0
                            hh_hi = 1
                        } else {
                            hh_lo++
                        }
                        
                        // Check for 11:59:59 -> 12:00:00 with PM toggle
                        if hh_reg == 0x11 && mm_reg == 0x59 && ss_reg == 0x59 {
                            pm_reg = !pm_reg
                        }
                    } else {
                        mm_hi++
                    }
                } else {
                    mm_lo++
                }
            } else {
                ss_hi++
            }
        } else {
            ss_lo++
        }
        
        // Reconstruct BCD values
        ss_reg = (ss_hi << 4) | ss_lo
        mm_reg = (mm_hi << 4) | mm_lo
        hh_reg = (hh_hi << 4) | hh_lo
        
        // Handle special case: when we go from 12:59:59 to 01:00:00
        // This happens after the increment logic above
        if hh_reg == 0x13 { // This would be 13 in BCD, which is invalid
            hh_reg = 0x01
        }
    }
    
    // Drive outputs
    out_pm = pm_reg
    out_hh = hh_reg
    out_mm = mm_reg
    out_ss = ss_reg
}

func main() {}
