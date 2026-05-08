package main

var out_left bool
var out_down bool
var out_right bool
var out_up bool

func TopModule(scancode uint16) {
    out_left = false
    out_down = false
    out_right = false
    out_up = false

    switch scancode {
    case 0xe06b:
        out_left = true
    case 0xe072:
        out_down = true
    case 0xe074:
        out_right = true
    case 0xe075:
        out_up = true
    }
}

func main() {}
