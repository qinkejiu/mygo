package main

var out_z bool

func TopModule(clk bool, reset bool, w bool) {
    // State encoding: A=0, B=1, C=2, D=3, E=4, F=5
    const (
        A = 0
        B = 1
        C = 2
        D = 3
        E = 4
        F = 5
    )
    
    // State register (3 bits to hold values 0-5)
    var state uint8 = A
    var next uint8 = A
    
    // State transition logic (combinational)
    switch state {
    case A:
        if w {
            next = B
        } else {
            next = A
        }
    case B:
        if w {
            next = C
        } else {
            next = D
        }
    case C:
        if w {
            next = E
        } else {
            next = D
        }
    case D:
        if w {
            next = F
        } else {
            next = A
        }
    case E:
        if w {
            next = E
        } else {
            next = D
        }
    case F:
        if w {
            next = C
        } else {
            next = D
        }
    default:
        next = A
    }
    
    // Sequential logic (positive edge triggered)
    if clk {
        if reset {
            state = A
        } else {
            state = next
        }
    }
    
    // Output logic: z = 1 when state is E or F
    out_z = (state == E) || (state == F)
}

func main() {}
