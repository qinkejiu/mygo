package main

var out_out bool

func TopModule(a bool, b bool, c bool, d bool) {
    // Combine inputs into a 4-bit value: {a,b,c,d} where a is MSB? 
    // In the Karnaugh map, ab are columns (a,b) and cd are rows (c,d).
    // Let's map: bits[3:0] = {a, b, c, d} where a is MSB, d is LSB.
    // Then we can use a switch on the 4-bit value.
    var bits uint8 = 0
    if a {
        bits |= 1 << 3
    }
    if b {
        bits |= 1 << 2
    }
    if c {
        bits |= 1 << 1
    }
    if d {
        bits |= 1 << 0
    }

    switch bits {
    case 0x0, 0x3, 0x5, 0x6, 0x9, 0xA, 0xC, 0xF:
        out_out = false
    case 0x1, 0x2, 0x4, 0x7, 0x8, 0xB, 0xD, 0xE:
        out_out = true
    }
}

func main() {}
