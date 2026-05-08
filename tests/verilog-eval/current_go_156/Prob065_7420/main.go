package main

var out_p1y bool
var out_p2y bool

func TopModule(p1a bool, p1b bool, p1c bool, p1d bool, p2a bool, p2b bool, p2c bool, p2d bool) {
    out_p1y = !(p1a && p1b && p1c && p1d)
    out_p2y = !(p2a && p2b && p2c && p2d)
}

func main() {}
