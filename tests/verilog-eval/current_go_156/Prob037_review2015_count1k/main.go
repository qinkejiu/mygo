package main

var out_q uint16

func TopModule(clk bool, reset bool) {
    // Use uint16 to hold values up to 999 (fits in 10 bits)
    static_counter := uint16(0)
    
    if clk {
        if reset || static_counter == 999 {
            static_counter = 0
        } else {
            static_counter = static_counter + 1
        }
    }
    
    out_q = static_counter
}

func main() {}
