package main

var out_out bool
var out_out_n bool

func TopModule(a bool, b bool, c bool, d bool) {
    w1 := a && b
    w2 := c && d
    out_out = w1 || w2
    out_out_n = !out_out
}

func main() {}
