package main

var out_and bool
var out_or bool
var out_xor bool
var out_nand bool
var out_nor bool
var out_xnor bool
var out_anotb bool

func TopModule(a bool, b bool) {
    out_and = a && b
    out_or = a || b
    out_xor = a != b
    out_nand = !(a && b)
    out_nor = !(a || b)
    out_xnor = a == b
    out_anotb = a && !b
}

func main() {}
