package main

var out_z bool

func TopModule(A uint8, B uint8) {
    out_z = (A & 0x3) == (B & 0x3)
}

func main() {}
