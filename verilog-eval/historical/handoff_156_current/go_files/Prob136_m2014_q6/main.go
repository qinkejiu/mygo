package main

var out_z bool

func TopModule(clk bool, reset bool, w bool) {
    // State encoding: A=0, B=1, C=2, D=3, E=4, F=5
    var state uint8
    var next uint8
    
    // Sequential logic
    if clk {
        if reset {
            state = 0 // A
        } else {
            state = next
        }
    }
    
    // Combinational next state logic
    switch state {
    case 0: // A
        if w {
            next = 0 // A
        } else {
            next = 1 // B
        }
    case 1: // B
        if w {
            next = 3 // D
        } else {
            next = 2 // C
        }
    case 2: // C
        if w {
            next = 3 // D
        } else {
            next = 4 // E
        }
    case 3: // D
        if w {
            next = 0 // A
        } else {
            next = 5 // F
        }
    case 4: // E
        if w {
            next = 3 // D
        } else {
            next = 4 // E
        }
    case 5: // F
        if w {
            next = 3 // D
        } else {
            next = 2 // C
        }
    default:
        next = 0 // A (safe default)
    }
    
    // Output logic: z = (state == E || state == F)
    out_z = (state == 4) || (state == 5)
}

func main() {}
