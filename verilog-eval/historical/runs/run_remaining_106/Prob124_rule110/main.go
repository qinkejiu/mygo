package main

var out_q [512]bool

func TopModule(clk bool, load bool, data [512]bool) {
    // Sequential logic triggered on positive edge of clk
    // We'll model this with a simple conditional check
    if clk {
        if load {
            // Load new data
            for i := 0; i < 512; i++ {
                out_q[i] = data[i]
            }
        } else {
            // Apply Rule 110 to compute next state
            var next_q [512]bool
            
            // Handle boundary conditions: left neighbor of q[0] is false, right neighbor of q[511] is false
            // Cell 0: left neighbor is false, center is q[0], right neighbor is q[1]
            left := false
            center := out_q[0]
            right := out_q[1]
            next_q[0] = rule110(left, center, right)
            
            // Cells 1 through 510: normal case with both neighbors
            for i := 1; i < 511; i++ {
                left = out_q[i+1]    // Note: left neighbor is actually q[i+1] based on the reference
                center = out_q[i]
                right = out_q[i-1]
                next_q[i] = rule110(left, center, right)
            }
            
            // Cell 511: left neighbor is q[510], center is q[511], right neighbor is false
            left = out_q[510]
            center = out_q[511]
            right = false
            next_q[511] = rule110(left, center, right)
            
            // Update output
            for i := 0; i < 512; i++ {
                out_q[i] = next_q[i]
            }
        }
    }
}

// Helper function to compute Rule 110
func rule110(left, center, right bool) bool {
    // Rule 110 truth table:
    // Left | Center | Right | Next
    // 1    | 1      | 1     | 0
    // 1    | 1      | 0     | 1
    // 1    | 0      | 1     | 1
    // 1    | 0      | 0     | 0
    // 0    | 1      | 1     | 1
    // 0    | 1      | 0     | 1
    // 0    | 0      | 1     | 1
    // 0    | 0      | 0     | 0
    
    // Convert to uint8 for easier comparison
    l := uint8(0)
    c := uint8(0)
    r := uint8(0)
    if left {
        l = 1
    }
    if center {
        c = 1
    }
    if right {
        r = 1
    }
    
    // Pack into a 3-bit value: left-center-right
    val := (l << 2) | (c << 1) | r
    
    // Apply Rule 110
    switch val {
    case 0b111: // 7
        return false
    case 0b110: // 6
        return true
    case 0b101: // 5
        return true
    case 0b100: // 4
        return false
    case 0b011: // 3
        return true
    case 0b010: // 2
        return true
    case 0b001: // 1
        return true
    case 0b000: // 0
        return false
    }
    return false // Should never reach here
}

func main() {}
