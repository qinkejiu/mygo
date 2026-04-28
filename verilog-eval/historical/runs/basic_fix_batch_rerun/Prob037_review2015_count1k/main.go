package main

var out_q uint16

func TopModule(clk bool, reset bool) {
    // Package-level variable to maintain counter state across calls
    var counter uint16 = 0
    
    // On positive edge of clock
    if clk {
        if reset {
            counter = 0
        } else if counter == 999 {
            counter = 0
        } else {
            counter = counter + 1
        }
    }
    
    // Always output the current counter value
    out_q = counter
}

func main() {}
