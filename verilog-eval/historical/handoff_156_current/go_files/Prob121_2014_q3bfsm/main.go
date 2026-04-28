package main

var out_z bool

func TopModule(clk bool, reset bool, x bool) {
    // State encoding: 000=0, 001=1, 010=2, 011=3, 100=4
    var state uint8 = 0
    var next uint8 = 0
    
    // Sequential logic
    if clk {
        if reset {
            state = 0
        } else {
            state = next
        }
    }
    
    // Combinational next state logic
    switch state {
    case 0: // 000
        if x {
            next = 1 // 001
        } else {
            next = 0 // 000
        }
    case 1: // 001
        if x {
            next = 4 // 100
        } else {
            next = 1 // 001
        }
    case 2: // 010
        if x {
            next = 1 // 001
        } else {
            next = 2 // 010
        }
    case 3: // 011
        if x {
            next = 2 // 010
        } else {
            next = 1 // 001
        }
    case 4: // 100
        if x {
            next = 4 // 100
        } else {
            next = 3 // 011
        }
    default:
        next = 0
    }
    
    // Output logic: z = (state == D) || (state == E)
    // D=3 (011), E=4 (100)
    out_z = (state == 3) || (state == 4)
}

func main() {}
