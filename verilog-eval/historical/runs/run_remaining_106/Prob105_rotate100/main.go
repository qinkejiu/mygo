package main

var out_q [100]bool

func TopModule(clk bool, load bool, ena uint8, data [100]bool) {
    // Internal register state
    var reg [100]bool

    // Initialize reg (simulate initial state)
    // In real hardware this would be a reset, but we'll handle it on clock edges
    // We'll use a simple approach: update reg only on positive clock edge
    
    // For simulation, we need to track previous clock state
    // Since we can't have persistent state across calls, we'll use a simple approach:
    // This function will be called every cycle, so we need to maintain state
    // We'll use a package-level variable to hold the register state
    // But per rules, we can only have out_* globals, so we'll use out_q as the state
    
    // On positive clock edge
    if clk {
        if load {
            // Load data into register
            for i := 0; i < 100; i++ {
                reg[i] = data[i]
            }
        } else if ena == 0x01 {
            // Rotate right by one bit
            // Save LSB
            lsb := out_q[0]
            // Shift right
            for i := 0; i < 99; i++ {
                reg[i] = out_q[i+1]
            }
            // Put saved LSB at MSB position
            reg[99] = lsb
        } else if ena == 0x02 {
            // Rotate left by one bit
            // Save MSB
            msb := out_q[99]
            // Shift left
            for i := 99; i > 0; i-- {
                reg[i] = out_q[i-1]
            }
            // Put saved MSB at LSB position
            reg[0] = msb
        } else {
            // No rotation, maintain current state
            for i := 0; i < 100; i++ {
                reg[i] = out_q[i]
            }
        }
        
        // Update output
        for i := 0; i < 100; i++ {
            out_q[i] = reg[i]
        }
    } else {
        // Not a clock edge, maintain current state
        // Output remains unchanged
    }
}

func main() {}
