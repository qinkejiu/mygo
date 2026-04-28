package main

var out_Q bool

func TopModule(clk bool, L bool, q_in bool, r_in bool) {
    if clk {
        if L {
            out_Q = r_in
        } else {
            out_Q = q_in
        }
    }
}

func main() {}
