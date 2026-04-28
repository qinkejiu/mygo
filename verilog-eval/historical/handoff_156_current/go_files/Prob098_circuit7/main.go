package main

var out_q bool

func TopModule(clk bool, a bool) {
    if clk {
        out_q = !a
    }
}

func main() {}
