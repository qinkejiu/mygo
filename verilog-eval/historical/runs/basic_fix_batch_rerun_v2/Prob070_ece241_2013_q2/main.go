package main

var out_sop bool
var out_pos bool

func TopModule(a bool, b bool, c bool, d bool) {
    // Sum-of-products: c&d | ~a&~b&c
    sop := (c && d) || (!a && !b && c)
    
    // Product-of-sums: c & (~b|d) & (~a|b) and c & (~b|d) & (~a|d)
    // Since both should be equal according to reference, we can compute one
    pos := c && (!b || d) && (!a || b)
    
    out_sop = sop
    out_pos = pos
}

func main() {}
