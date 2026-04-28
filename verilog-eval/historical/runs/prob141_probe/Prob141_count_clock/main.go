package main

var out_pm bool
var out_hh uint8
var out_mm uint8
var out_ss uint8

func TopModule(clk bool, reset bool, ena bool) {
    // Sequential logic triggered on positive edge of clk
    // We'll model this with a simple conditional check
    // In real hardware, this would be edge-triggered flip-flops
    
    // Internal state registers
    var pm_reg bool
    var hh_reg uint8
    var mm_reg uint8
    var ss_reg uint8
    
    // Initialize to reset state (12:00 AM)
    if reset {
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
        
        // Determine which counters need to increment
        enable_ss_lo := true
        enable_ss_hi := ss_lo == 9
        enable_mm_lo := enable_ss_hi && ss_hi == 5
        enable_mm_hi := enable_mm_lo && mm_lo == 9
        enable_hh_lo := enable_mm_hi && mm_hi == 5
        enable_hh_hi := enable_hh_lo && hh_lo == 9
        toggle_pm := enable_hh_hi && hh_hi == 1 && hh_lo == 2
        
        // Update seconds (00-59)
        if enable_ss_lo {
            if ss_lo == 9 {
                ss_lo = 0
            } else {
                ss_lo++
            }
        }
        
        if enable_ss_hi {
            if ss_hi == 5 {
                ss_hi = 0
            } else {
                ss_hi++
            }
        }
        
        // Update minutes (00-59)
        if enable_mm_lo {
            if mm_lo == 9 {
                mm_lo = 0
            } else {
                mm_lo++
            }
        }
        
        if enable_mm_hi {
            if mm_hi == 5 {
                mm_hi = 0
            } else {
                mm_hi++
            }
        }
        
        // Update hours (01-12)
        if enable_hh_lo {
            if hh_lo == 9 {
                hh_lo = 0
            } else {
                hh_lo++
            }
        }
        
        if enable_hh_hi {
            if hh_hi == 1 && hh_lo == 2 {
                // Special case: 12 -> 01
                hh_hi = 0
                hh_lo = 1
            } else if hh_hi == 0 && hh_lo == 9 {
                // 09 -> 10
                hh_hi = 1
                hh_lo = 0
            } else if hh_hi == 1 && hh_lo == 1 {
                // 11 -> 12
                hh_hi = 1
                hh_lo = 2
            }
        }
        
        // Toggle PM/AM at 12:00:00
        if toggle_pm {
            pm_reg = !pm_reg
        }
        
        // Recombine BCD digits
        ss_reg = (ss_hi << 4) | ss_lo
        mm_reg = (mm_hi << 4) | mm_lo
        hh_reg = (hh_hi << 4) | hh_lo
    }
    
    // Assign outputs
    out_pm = pm_reg
    out_hh = hh_reg
    out_mm = mm_reg
    out_ss = ss_reg
}

func main() {}
