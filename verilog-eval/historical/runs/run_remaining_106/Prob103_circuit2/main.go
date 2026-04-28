package main

var out_q bool

func TopModule(a bool, b bool, c bool, d bool) {
    out_q = (a != b) != (c != d)
}

func main() {}
