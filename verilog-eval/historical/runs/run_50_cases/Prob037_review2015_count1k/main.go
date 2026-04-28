package main

var out_q uint16

func TopModule(clk bool, reset bool) {
    // Use a 10-bit counter (0-999 fits in 10 bits, 2^10=1024)
    // We'll use uint16 for convenience since it's the next standard size
    // and can hold values up to 999 easily
    
    // This is a sequential design, so we need to model state
    // In real hardware this would be a register, here we use a package variable
    // to maintain state across calls (simulating clock cycles)
    var counter uint16
    
    // Since we can't have persistent state across TopModule calls in pure combinational logic,
    // and the prompt shows a clocked design, we'll implement the logic as if
    // TopModule is called on each clock edge with the current state.
    // In practice, a testbench would call TopModule repeatedly with clock edges.
    
    // For a complete implementation, we need to track state.
    // We'll use a package variable to simulate the register.
    // This follows the pattern of using package globals for outputs.
    var reg_counter uint16 = 0
    
    // On positive clock edge (when clk is true in this cycle)
    // In real simulation, we'd compare with previous clock value,
    // but for this simple model, we'll assume TopModule is called
    // only on clock edges or we detect the edge internally.
    // We'll implement the logic directly:
    if clk {
        if reset || reg_counter == 999 {
            reg_counter = 0
        } else {
            reg_counter = reg_counter + 1
        }
    }
    
    // Update the output
    out_q = reg_counter
}

func main() {}
