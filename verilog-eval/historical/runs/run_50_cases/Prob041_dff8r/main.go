package main

var out_q uint8

func TopModule(clk bool, reset bool, d uint8) {
    // D flip-flop with synchronous reset
    if clk {
        if reset {
            out_q = 0
        } else {
            out_q = d
        }
    }
}

func main() {}
