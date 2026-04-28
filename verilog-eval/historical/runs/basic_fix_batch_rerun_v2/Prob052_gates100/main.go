package main

var out_and bool
var out_or bool
var out_xor bool

func TopModule(in [100]bool) {
    // Initialize outputs
    andResult := true
    orResult := false
    xorResult := false
    
    // Process each bit
    for i := 0; i < 100; i++ {
        bit := in[i]
        
        // AND: all bits must be true
        andResult = andResult && bit
        
        // OR: at least one bit must be true
        orResult = orResult || bit
        
        // XOR: parity of bits
        xorResult = xorResult != bit
    }
    
    // Assign to output globals
    out_and = andResult
    out_or = orResult
    out_xor = xorResult
}

func main() {}
