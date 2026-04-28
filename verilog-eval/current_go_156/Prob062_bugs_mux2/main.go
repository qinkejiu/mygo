package main

var out_out uint8

func TopModule(sel bool, a uint8, b uint8) {
    if sel {
        out_out = a
    } else {
        out_out = b
    }
}

func main() {}
