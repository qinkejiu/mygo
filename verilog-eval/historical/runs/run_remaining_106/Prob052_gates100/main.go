package main

var out_and bool
var out_or bool
var out_xor bool

func TopModule(in [100]bool) {
    // Initialize with first bit
    and_result := in[0]
    or_result := in[0]
    xor_result := in[0]
    
    // Process remaining 99 bits
    for i := 1; i < 100; i++ {
        and_result = and_result && in[i]
        or_result = or_result || in[i]
        xor_result = xor_result != in[i]
    }
    
    out_and = and_result
    out_or = or_result
    out_xor = xor_result
}

func main() {}
