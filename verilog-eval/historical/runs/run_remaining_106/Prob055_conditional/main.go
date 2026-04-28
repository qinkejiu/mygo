package main

var out_min uint8

func TopModule(a uint8, b uint8, c uint8, d uint8) {
    out_min = a
    if out_min > b {
        out_min = b
    }
    if out_min > c {
        out_min = c
    }
    if out_min > d {
        out_min = d
    }
}

func main() {}
