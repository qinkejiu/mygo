package main

var out_q bool

func TopModule(clk bool, d bool, r bool) {
    // D flip-flop with synchronous reset
    // Reset has priority: when r is true on clock edge, q becomes false
    // Otherwise, q takes the value of d
    // This is a clocked sequential element
    // In MyGO, we model this with a conditional check on clk
    // Since we can't detect edges directly, we'll assume TopModule is called
    // on every clock cycle and the logic represents the behavior at the posedge
    if clk {
        if r {
            out_q = false
        } else {
            out_q = d
        }
    }
    // If not clk, out_q retains its previous value (implicit in hardware)
}

func main() {}
