package main

var out_out bool

func TopModule(a bool, b bool, c bool, d bool) {
    // Combine inputs into a 4-bit value: {a,b,c,d} where a is MSB? 
    // Based on the Karnaugh map and Verilog reference, the mapping is:
    // {a,b,c,d} corresponds to ab as columns (01,00,10,11) and cd as rows (00,01,11,10)
    // Let's interpret the mapping from the truth table.
    // We'll implement the logic directly from the Karnaugh map.
    
    // From the map:
    // When cd=00: out = 0 when ab=01, 0 when ab=00, 1 when ab=10, 1 when ab=11
    // When cd=01: out = 0 when ab=01, 0 when ab=00, d when ab=10, d when ab=11
    // When cd=11: out = 0 when ab=01, 1 when ab=00, 1 when ab=10, 1 when ab=11
    // When cd=10: out = 0 when ab=01, 1 when ab=00, 1 when ab=10, 1 when ab=11
    
    // We can simplify by observing patterns.
    // Let's encode ab and cd as 2-bit values:
    // ab: 00->0, 01->1, 10->2, 11->3
    // cd: 00->0, 01->1, 11->3, 10->2
    
    // Convert bools to bits for easier computation
    ab := 0
    if a { ab |= 2 } // a is high bit of ab pair
    if b { ab |= 1 } // b is low bit of ab pair
    
    cd := 0
    if c { cd |= 2 } // c is high bit of cd pair
    if d { cd |= 1 } // d is low bit of cd pair
    
    // Determine output based on cd and ab
    switch cd {
    case 0: // cd=00
        switch ab {
        case 0: // ab=00
            out_out = false
        case 1: // ab=01
            out_out = false
        case 2: // ab=10
            out_out = true
        case 3: // ab=11
            out_out = true
        }
    case 1: // cd=01
        switch ab {
        case 0: // ab=00
            out_out = false
        case 1: // ab=01
            out_out = false
        case 2: // ab=10
            // d is don't-care, choose convenient value
            // We'll choose false for simplicity
            out_out = false
        case 3: // ab=11
            // d is don't-care, choose convenient value
            // We'll choose false for simplicity
            out_out = false
        }
    case 3: // cd=11
        switch ab {
        case 0: // ab=00
            out_out = true
        case 1: // ab=01
            out_out = false
        case 2: // ab=10
            out_out = true
        case 3: // ab=11
            out_out = true
        }
    case 2: // cd=10
        switch ab {
        case 0: // ab=00
            out_out = true
        case 1: // ab=01
            out_out = false
        case 2: // ab=10
            out_out = true
        case 3: // ab=11
            out_out = true
        }
    }
}

func main() {}
