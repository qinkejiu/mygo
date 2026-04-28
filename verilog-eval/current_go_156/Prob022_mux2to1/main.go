package main

var out_out bool

func TopModule(a bool, b bool, sel bool) {
    if sel {
        out_out = b
    } else {
        out_out = a
    }
}

func main() {}
