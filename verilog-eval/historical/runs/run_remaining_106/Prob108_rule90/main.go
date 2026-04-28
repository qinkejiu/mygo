package main

var out_q [512]bool

func TopModule(clk bool, load bool, data [512]bool) {
    if clk {
        if load {
            for i := 0; i < 512; i++ {
                out_q[i] = data[i]
            }
        } else {
            var next [512]bool
            for i := 0; i < 512; i++ {
                left := false
                right := false
                
                if i > 0 {
                    left = out_q[i-1]
                }
                if i < 511 {
                    right = out_q[i+1]
                }
                
                next[i] = left != right
            }
            out_q = next
        }
    }
}

func main() {}
