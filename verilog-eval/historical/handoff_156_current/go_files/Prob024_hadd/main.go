package main

var out_sum bool
var out_cout bool

func TopModule(a bool, b bool) {
    out_sum = a != b
    out_cout = a && b
}

func main() {}
