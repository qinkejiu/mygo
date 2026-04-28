package main

var out_walk_left bool
var out_walk_right bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool) {
    // State encoding: false = WL (walk left), true = WR (walk right)
    var state bool
    var next bool
    
    // Sequential logic
    if areset {
        state = false // Reset to walk left
    } else if clk {
        state = next
    }
    
    // Combinational next state logic
    if !state { // WL state
        if bump_left {
            next = true // Switch to WR
        } else {
            next = false // Stay in WL
        }
    } else { // WR state
        if bump_right {
            next = false // Switch to WL
        } else {
            next = true // Stay in WR
        }
    }
    
    // Output logic (Moore outputs)
    out_walk_left = !state
    out_walk_right = state
}

func main() {}
