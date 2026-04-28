package main

var out_ena uint8
var out_q uint16

func TopModule(clk bool, reset bool) {
    // Sequential logic triggered on positive edge of clk
    // We'll model this with a simple if statement checking clk
    // In real hardware, this would be edge-triggered
    
    // We need to maintain state across calls, so we use package-level variables
    // to store the current counter value
    var current_q uint16
    
    // For simulation purposes, we'll check if clk is true (positive edge)
    // In a real MyGO implementation, this would be handled differently
    if clk {
        if reset {
            current_q = 0
        } else {
            // Extract BCD digits
            ones := current_q & 0xF
            tens := (current_q >> 4) & 0xF
            hundreds := (current_q >> 8) & 0xF
            thousands := (current_q >> 12) & 0xF
            
            // Calculate enable signals
            enable0 := true  // Always enable ones digit
            enable1 := (ones == 9)
            enable2 := (ones == 9) && (tens == 9)
            enable3 := (ones == 9) && (tens == 9) && (hundreds == 9)
            
            // Update digits based on enable signals
            if enable0 {
                if ones == 9 {
                    ones = 0
                } else {
                    ones++
                }
            }
            
            if enable1 {
                if tens == 9 {
                    tens = 0
                } else {
                    tens++
                }
            }
            
            if enable2 {
                if hundreds == 9 {
                    hundreds = 0
                } else {
                    hundreds++
                }
            }
            
            if enable3 {
                if thousands == 9 {
                    thousands = 0
                } else {
                    thousands++
                }
            }
            
            // Reconstruct q
            current_q = (thousands << 12) | (hundreds << 8) | (tens << 4) | ones
        }
        
        // Set outputs
        out_q = current_q
        
        // Calculate ena[3:1] - enable signals for digits 3, 2, and 1
        // ena[1] corresponds to tens digit, ena[2] to hundreds, ena[3] to thousands
        ones := current_q & 0xF
        tens := (current_q >> 4) & 0xF
        hundreds := (current_q >> 8) & 0xF
        
        ena1 := (ones == 9)
        ena2 := (ones == 9) && (tens == 9)
        ena3 := (ones == 9) && (tens == 9) && (hundreds == 9)
        
        // Pack enable signals into 3-bit output
        out_ena = 0
        if ena1 {
            out_ena |= 0x1
        }
        if ena2 {
            out_ena |= 0x2
        }
        if ena3 {
            out_ena |= 0x4
        }
    }
}

func main() {}
