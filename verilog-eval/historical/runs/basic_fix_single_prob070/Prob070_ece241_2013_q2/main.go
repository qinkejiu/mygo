package main

var out_sop bool
var out_pos bool

func TopModule(a bool, b bool, c bool, d bool) {
    // Sum-of-products: c&d | ~a&~b&c
    sop := (c && d) || (!a && !b && c)
    
    // Product-of-sums: c & (~b|d) & (~a|b) & (~a|d)
    // First compute the three OR terms
    term1 := !b || d
    term2 := !a || b
    term3 := !a || d
    
    // Then AND them all with c
    pos := c && term1 && term2 && term3
    
    out_sop = sop
    out_pos = pos
}

func main() {}
