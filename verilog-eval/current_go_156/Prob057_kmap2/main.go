package main

var out_out bool

func TopModule(a bool, b bool, c bool, d bool) {
    // Karnaugh map implementation based on the provided map
    // Map coordinates:
    // ab columns: 00, 01, 11, 10
    // cd rows: 00, 01, 11, 10
    
    // From the reference Verilog: out = (~c & ~b) | (~d & ~a) | (a & c & d) | (b & c & d)
    
    // Calculate each term
    term1 := (!c) && (!b)  // ~c & ~b
    term2 := (!d) && (!a)  // ~d & ~a
    term3 := a && c && d   // a & c & d
    term4 := b && c && d   // b & c & d
    
    // Combine terms with OR
    out_out = term1 || term2 || term3 || term4
}

func main() {}
