package main

var out_sop bool
var out_pos bool

func TopModule(a bool, b bool, c bool, d bool) {
    // Sum-of-products: c&d | ~a&~b&c
    sop := (c && d) || (!a && !b && c)
    
    // Product-of-sums: c & (~b|d) & (~a|b) & (~a|d)
    // First POS expression: c & (~b|d) & (~a|b)
    pos0 := c && (!b || d) && (!a || b)
    
    // Second POS expression: c & (~b|d) & (~a|d)
    pos1 := c && (!b || d) && (!a || d)
    
    // Both POS expressions should be equal for valid inputs
    // For invalid inputs (3, 8, 11, 12), we don't care
    out_sop = sop
    out_pos = pos0 // pos0 and pos1 are equal for valid inputs
}

func main() {}
