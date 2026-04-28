package main

var out_out [2]bool

func TopModule(in [3]bool) {
    // Count number of '1's in the input
    count := 0
    
    if in[0] {
        count++
    }
    if in[1] {
        count++
    }
    if in[2] {
        count++
    }
    
    // Convert count to 2-bit binary representation
    out_out[0] = (count & 1) != 0
    out_out[1] = (count & 2) != 0
}

func main() {}
