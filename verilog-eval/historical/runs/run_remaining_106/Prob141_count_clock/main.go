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
        
        // Determine which counters should increment
        enable_ss_lo := true  // enable[0]
        enable_ss_hi := (ss_lo == 0x9)  // enable[1]
        enable_mm_lo := (ss_lo == 0x9) && (ss_hi == 0x5)  // enable[2]
        enable_mm_hi := (ss_lo == 0x9) && (ss_hi == 0x5) && (mm_lo == 0x9)  // enable[3]
        enable_hh_lo := (ss_lo == 0x9) && (ss_hi == 0x5) && (mm_lo == 0x9) && (mm_hi == 0x5)  // enable[4]
        enable_hh_hi := (ss_lo == 0x9) && (ss_hi == 0x5) && (mm_lo == 0x9) && (mm_hi == 0x5) && (hh_lo == 0x9)  // enable[5]
        toggle_pm := (ss_lo == 0x9) && (ss_hi == 0x5) && (mm_lo == 0x9) && (mm_hi == 0x5) && (hh_reg == 0x11)  // enable[6] when hh=11
        
        // Update seconds (low digit)
        if enable_ss_lo {
            if ss_lo == 0x9 {
                ss_lo = 0x0
            } else {
                ss_lo++
            }
        }
        
        // Update seconds (high digit)
        if enable_ss_hi {
            if ss_hi == 0x5 {
                ss_hi = 0x0
            } else {
                ss_hi++
            }
        }
        
        // Update minutes (low digit)
        if enable_mm_lo {
            if mm_lo == 0x9 {
                mm_lo = 0x0
            } else {
                mm_lo++
            }
        }
        
        // Update minutes (high digit)
        if enable_mm_hi {
            if mm_hi == 0x5 {
                mm_hi = 0x0
            } else {
                mm_hi++
            }
        }
        
        // Update hours (low digit)
        if enable_hh_lo {
            if hh_lo == 0x9 {
                hh_lo = 0x0
            } else {
                hh_lo++
            }
        }
        
        // Update hours (high digit) and handle 12-hour rollover
        if enable_hh_hi {
            if hh_hi == 0x1 && hh_lo == 0x2 {
                // Roll from 12 to 01
                hh_hi = 0x0
                hh_lo = 0x1
            } else if hh_hi == 0x0 && hh_lo == 0x9 {
                // Roll from 09 to 10
                hh_hi = 0x1
                hh_lo = 0x0
            } else if hh_hi == 0x1 && hh_lo == 0x1 {
                // Roll from 11 to 12
                hh_hi = 0x1
                hh_lo = 0x2
            }
        } else if enable_hh_lo {
            // Special case: when going from 09 to 10
            if hh_hi == 0x0 && hh_lo == 0x9 {
                hh_hi = 0x1
                hh_lo = 0x0
            }
            // Special case: when going from 12 to 01
            if hh_reg == 0x12 {
                hh_hi = 0x0
                hh_lo = 0x1
            }
        }
        
        // Toggle PM/AM at 11:59:59
        if toggle_pm {
            pm_reg = !pm_reg
        }
        
        // Recombine BCD digits
        ss_reg = (ss_hi << 4) | ss_lo
        mm_reg = (mm_hi << 4) | mm_lo
        hh_reg = (hh_hi << 4) | hh_lo
    }
    
    // Drive outputs
    out_pm = pm_reg
    out_hh = hh_reg
    out_mm = mm_reg
    out_ss = ss_reg
}

func main() {}
