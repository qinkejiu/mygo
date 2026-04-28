package main

var out_both [100]bool
var out_any [100]bool
var out_different [100]bool

func TopModule(in [100]bool) {
    // out_both[99] = 0 (no neighbor to the left)
    out_both[99] = false
    
    // out_both[98:0] = in[98:0] & in[99:1]
    for i := 0; i < 99; i++ {
        out_both[i] = in[i] && in[i+1]
    }
    
    // out_any[0] = 0 (no neighbor to the right)
    out_any[0] = false
    
    // out_any[99:1] = in[98:0] | in[99:1]
    for i := 1; i < 100; i++ {
        out_any[i] = in[i] || in[i-1]
    }
    
    // out_different = in ^ {in[0], in[99:1]}
    // Handle wrap-around: in[99]'s left neighbor is in[0]
    for i := 0; i < 100; i++ {
        leftNeighbor := in[(i+1)%100] // For i=99, leftNeighbor = in[0]
        out_different[i] = in[i] != leftNeighbor
    }
}

func main() {}
