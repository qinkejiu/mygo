package main

var out_sop bool
var out_pos bool

func TopModule(a bool, b bool, c bool, d bool) {
    // Sum-of-products: c&d | ~a&~b&c
    sop := (c && d) || (!a && !b && c)
    
    // Product-of-sums: c & (~b|d) & (~a|b) & (~a|d)
    // First compute the two product-of-sums forms from reference
    pos0 := c && (!b || d) && (!a || b)
    pos1 := c && (!b || d) && (!a || d)
    
    // Since pos0 and pos1 should be equal for valid inputs
    // and we're told 3,8,11,12 never occur, we can use either
    pos := pos0 // or pos1, they should be equal
    
    out_sop = sop
    out_pos = pos
}

func main() {}
