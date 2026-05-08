package main

var out_q uint8

func TopModule(clk bool, reset bool) {
    // Internal state variable for the counter
    var counter uint8 = 1
    
    // Sequential logic triggered on positive clock edge
    if clk {
        if reset || counter == 10 {
            counter = 1
        } else {
            counter = counter + 1
        }
    }
    
    // Assign to output global
    out_q = counter
}

func main() {}
