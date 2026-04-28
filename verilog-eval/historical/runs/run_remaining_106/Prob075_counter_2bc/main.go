package main

var out_state uint8

func TopModule(clk bool, areset bool, train_valid bool, train_taken bool) {
    // Sequential logic triggered on positive edge of clk
    // We'll model this with a static variable to hold the current state
    var currentState uint8 = 1 // Default initial value (weakly not-taken)
    
    // This simulates the positive edge triggered behavior
    // In real hardware, this would be clocked logic
    if areset {
        currentState = 1 // Reset to 2'b01
    } else if train_valid {
        if train_taken && currentState < 3 {
            currentState = currentState + 1
        } else if !train_taken && currentState > 0 {
            currentState = currentState - 1
        }
    }
    
    out_state = currentState
}

func main() {}
