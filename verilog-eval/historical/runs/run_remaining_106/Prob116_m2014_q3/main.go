package main

var out_f bool

func TopModule(x uint8) {
    // x[3:0] corresponds to bits: x[3]=MSB, x[0]=LSB
    // Map to Karnaugh map variables:
    // x[3] = x3, x[2] = x2, x[1] = x1, x[0] = x0
    // Karnaugh map uses x3x4 for rows, x1x2 for columns
    // In our mapping: x3 = x[3], x2 = x[2], x1 = x[1], x0 = x[0]
    
    // Extract individual bits
    x3 := (x >> 3) & 0x1
    x2 := (x >> 2) & 0x1
    x1 := (x >> 1) & 0x1
    x0 := x & 0x1
    
    // Determine output based on Karnaugh map with don't-cares optimized
    // We'll implement the logic directly from the truth table
    switch x {
    case 0x0: // 0000 - don't care, choose 0
        out_f = false
    case 0x1: // 0001 - don't care, choose 0
        out_f = false
    case 0x2: // 0010
        out_f = false
    case 0x3: // 0011 - don't care, choose 1
        out_f = true
    case 0x4: // 0100
        out_f = true
    case 0x5: // 0101 - don't care, choose 0
        out_f = false
    case 0x6: // 0110
        out_f = true
    case 0x7: // 0111
        out_f = false
    case 0x8: // 1000
        out_f = false
    case 0x9: // 1001
        out_f = false
    case 0xa: // 1010 - don't care, choose 1
        out_f = true
    case 0xb: // 1011
        out_f = true
    case 0xc: // 1100
        out_f = true
    case 0xd: // 1101 - don't care, choose 1
        out_f = true
    case 0xe: // 1110
        out_f = true
    case 0xf: // 1111 - don't care, choose 1
        out_f = true
    default:
        out_f = false // Should never happen with 4-bit input
    }
}

func main() {}
