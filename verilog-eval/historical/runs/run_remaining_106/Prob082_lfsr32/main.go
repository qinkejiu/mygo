package main

var out_q uint32

func TopModule(clk bool, reset bool) {
    // Sequential logic triggered on positive edge of clk
    // We'll model this with a static variable to hold state between calls
    // In real hardware this would be a register
    var q_reg uint32 = 1 // Initial state after reset
    
    // This function gets called every cycle in simulation
    // We need to track state across calls
    // For MyGO, we'll use a package-level variable to hold the register state
    // between invocations of TopModule
    var q_next uint32
    
    // Calculate next state
    if reset {
        q_next = 1
    } else {
        // Galois LFSR logic with taps at positions 32, 22, 2, and 1
        // Note: In Go, bit positions are 0-indexed from LSB
        // So position 32 corresponds to bit 31 (MSB), position 22 is bit 21, etc.
        
        // Shift right by 1: q[31:1] becomes q_next[30:0]
        q_next = q_reg >> 1
        
        // Set MSB (bit 31) to q[0]
        if (q_reg & 0x1) != 0 {
            q_next |= 0x80000000 // Set bit 31
        } else {
            q_next &= 0x7FFFFFFF // Clear bit 31
        }
        
        // XOR tap at position 22 (bit 21) with q[0]
        if (q_reg & 0x1) != 0 {
            q_next ^= 0x00200000 // Toggle bit 21
        }
        
        // XOR tap at position 2 (bit 1) with q[0]
        if (q_reg & 0x1) != 0 {
            q_next ^= 0x00000002 // Toggle bit 1
        }
        
        // XOR tap at position 1 (bit 0) with q[0]
        if (q_reg & 0x1) != 0 {
            q_next ^= 0x00000001 // Toggle bit 0
        }
    }
    
    // Update register on positive clock edge
    if clk {
        q_reg = q_next
    }
    
    // Drive output
    out_q = q_reg
}

func main() {}
