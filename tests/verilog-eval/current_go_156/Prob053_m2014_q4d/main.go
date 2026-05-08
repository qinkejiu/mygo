package main

var out_out bool

func TopModule(clk bool, in bool) {
    // D flip-flop with no reset, positive edge triggered
    // The D input is in XOR out_out
    if clk {
        // On positive edge of clk (clk == true)
        out_out = in != out_out // XOR operation
    }
    // No else needed - on non-positive edge, out_out retains its value
}

func main() {}
