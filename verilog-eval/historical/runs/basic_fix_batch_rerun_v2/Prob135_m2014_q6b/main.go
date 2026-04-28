package main

var out_Y1 bool

func TopModule(y uint8, w bool) {
    out_Y1 = (y & 0x2) != 0
}

func main() {}
