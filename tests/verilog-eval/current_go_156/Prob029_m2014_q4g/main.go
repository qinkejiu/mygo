package main

var out_out bool

func TopModule(in1 bool, in2 bool, in3 bool) {
    xnorResult := !(in1 != in2)
    out_out = xnorResult != in3
}

func main() {}
