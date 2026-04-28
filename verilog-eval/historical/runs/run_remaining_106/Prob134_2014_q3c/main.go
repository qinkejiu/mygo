package main

var out_Y0 bool
var out_z bool

func TopModule(clk bool, x bool, y uint8) {
    // Extract individual bits from y (3-bit input)
    y0 := (y & 0x1) != 0
    y1 := (y & 0x2) != 0
    y2 := (y & 0x4) != 0

    // Determine next state Y[0] based on present state y[2:0] and input x
    var Y0_next bool
    switch y {
    case 0x0: // 000
        if x {
            Y0_next = true // 001 -> LSB=1
        } else {
            Y0_next = false // 000 -> LSB=0
        }
    case 0x1: // 001
        if x {
            Y0_next = false // 100 -> LSB=0
        } else {
            Y0_next = true // 001 -> LSB=1
        }
    case 0x2: // 010
        if x {
            Y0_next = true // 001 -> LSB=1
        } else {
            Y0_next = false // 010 -> LSB=0
        }
    case 0x3: // 011
        if x {
            Y0_next = false // 010 -> LSB=0
        } else {
            Y0_next = true // 001 -> LSB=1
        }
    case 0x4: // 100
        if x {
            Y0_next = false // 100 -> LSB=0
        } else {
            Y0_next = true // 011 -> LSB=1
        }
    default:
        Y0_next = false // default case
    }

    // Determine output z based on present state y[2:0]
    var z_val bool
    switch y {
    case 0x0, 0x1, 0x2:
        z_val = false
    case 0x3, 0x4:
        z_val = true
    default:
        z_val = false
    }

    // Clocked behavior: update outputs on clock edge
    // Assuming positive edge triggered
    // In MyGO, we model this with a conditional on clk
    // For combinational version, we assign directly
    out_Y0 = Y0_next
    out_z = z_val
}

func main() {}
