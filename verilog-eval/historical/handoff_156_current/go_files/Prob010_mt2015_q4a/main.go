package main

var out_z bool

func TopModule(x bool, y bool) {
    out_z = (x != y) && x
}

func main() {}
