package main

var out_Y0 bool
var out_z bool

func TopModule(clk bool, x bool, y uint8) {
    // Extract y bits (3-bit input)
    y2 := (y >> 2) & 0x1
    y1 := (y >> 1) & 0x1
    y0 := y & 0x1

    // Next state logic based on table
    var Y0_next bool

    switch y {
    case 0x0: // 000
        if x {
            Y0_next = true // 001
        } else {
            Y0_next = false // 000
        }
    case 0x1: // 001
        if x {
            Y0_next = false // 100
        } else {
            Y0_next = true // 001
        }
    case 0x2: // 010
        if x {
            Y0_next = true // 001
        } else {
            Y0_next = false // 010
        }
    case 0x3: // 011
        if x {
            Y0_next = false // 010
        } else {
            Y0_next = true // 001
        }
    case 0x4: // 100
        if x {
            Y0_next = false // 100
        } else {
            Y0_next = true // 011
        }
    default:
        // For undefined states, keep same state
        Y0_next = y0 != 0
    }

    // Output logic from table
    switch y {
    case 0x0, 0x1, 0x2:
        out_z = false
    case 0x3, 0x4:
        out_z = true
    default:
        out_z = false
    }

    // Clocked behavior (simplified - just assign next state output)
    // In real hardware this would be registered on clk edge
    out_Y0 = Y0_next
}

func main() {}
