package main

var out_Y1 bool

func TopModule(y uint8, w bool) {
    // Extract individual bits from y (3-bit input)
    // y[0] is LSB, y[1] is middle bit, y[2] is MSB
    y0 := (y & 0x1) != 0
    y1 := (y & 0x2) != 0
    y2 := (y & 0x4) != 0
    
    // State encoding: y[2:0] = {y2, y1, y0}
    // A: 000, B: 001, C: 010, D: 011, E: 100, F: 101
    
    // Next-state logic for y[1] only
    // Based on the state transition table and encoding
    next_y1 := false
    
    // Determine current state from y bits
    switch {
    case !y2 && !y1 && !y0: // A (000)
        if w {
            next_y1 = false // A->A: y1 stays 0
        } else {
            next_y1 = false // A->B: y1 stays 0
        }
    case !y2 && !y1 && y0: // B (001)
        if w {
            next_y1 = true  // B->D: y1 becomes 1
        } else {
            next_y1 = true  // B->C: y1 becomes 1
        }
    case !y2 && y1 && !y0: // C (010)
        if w {
            next_y1 = true  // C->D: y1 stays 1
        } else {
            next_y1 = false // C->E: y1 becomes 0
        }
    case !y2 && y1 && y0: // D (011)
        if w {
            next_y1 = false // D->A: y1 becomes 0
        } else {
            next_y1 = true  // D->F: y1 stays 1
        }
    case y2 && !y1 && !y0: // E (100)
        if w {
            next_y1 = true  // E->D: y1 becomes 1
        } else {
            next_y1 = false // E->E: y1 stays 0
        }
    case y2 && !y1 && y0: // F (101)
        if w {
            next_y1 = true  // F->D: y1 becomes 1
        } else {
            next_y1 = true  // F->C: y1 becomes 1
        }
    default:
        // Should not happen for valid 3-bit y input
        next_y1 = false
    }
    
    // Output Y1 is simply y[1] (current state bit)
    out_Y1 = y1
}

func main() {}
