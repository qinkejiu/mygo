package main

var out_out [100]bool

func TopModule(a [100]bool, b [100]bool, sel bool) {
    if sel {
        out_out = b
    } else {
        out_out = a
    }
}

func main() {}
