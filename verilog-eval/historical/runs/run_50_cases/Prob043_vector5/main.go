package main

var out_out uint32

func TopModule(a bool, b bool, c bool, d bool, e bool) {
    // Pack the 5 input bits into a uint8 for easier manipulation
    var inputs uint8 = 0
    if a {
        inputs |= 1 << 0
    }
    if b {
        inputs |= 1 << 1
    }
    if c {
        inputs |= 1 << 2
    }
    if d {
        inputs |= 1 << 3
    }
    if e {
        inputs |= 1 << 4
    }

    // Generate the 25-bit comparison result
    // We'll use uint32 to hold the 25-bit result (fits in 32 bits)
    var result uint32 = 0

    // Create the two 25-bit vectors as described in the reference Verilog
    // First vector: {5{a}}, {5{b}}, {5{c}}, {5{d}}, {5{e}}
    // Second vector: {5{a,b,c,d,e}}
    
    // Build first vector: 5 copies of each input
    var firstVec uint32 = 0
    for i := 0; i < 5; i++ {
        // Add 5 copies of a
        if a {
            firstVec |= 1 << (24 - i)
        }
        // Add 5 copies of b
        if b {
            firstVec |= 1 << (19 - i)
        }
        // Add 5 copies of c
        if c {
            firstVec |= 1 << (14 - i)
        }
        // Add 5 copies of d
        if d {
            firstVec |= 1 << (9 - i)
        }
        // Add 5 copies of e
        if e {
            firstVec |= 1 << (4 - i)
        }
    }

    // Build second vector: {a,b,c,d,e} repeated 5 times
    var secondVec uint32 = 0
    for i := 0; i < 5; i++ {
        // First group: a,b,c,d,e
        if a {
            secondVec |= 1 << (24 - (i*5 + 0))
        }
        if b {
            secondVec |= 1 << (24 - (i*5 + 1))
        }
        if c {
            secondVec |= 1 << (24 - (i*5 + 2))
        }
        if d {
            secondVec |= 1 << (24 - (i*5 + 3))
        }
        if e {
            secondVec |= 1 << (24 - (i*5 + 4))
        }
    }

    // Compute out = ~firstVec ^ secondVec
    // Note: In Go, ~ is bitwise NOT, but it operates on all bits
    // We need to mask to 25 bits
    result = (^firstVec & 0x1FFFFFF) ^ secondVec
    
    // Alternative direct computation for clarity:
    // We can compute each bit directly as specified in the prompt
    result = 0
    
    // Generate all 25 pairwise comparisons
    // out[24] = ~a ^ a = 1 (always)
    // out[23] = ~a ^ b
    // out[22] = ~a ^ c
    // ...
    // out[0] = ~e ^ e = 1 (always)
    
    // Inputs array for easier indexing
    inputsArr := [5]bool{a, b, c, d, e}
    
    bitPos := 24
    for i := 0; i < 5; i++ {
        for j := 0; j < 5; j++ {
            // Compute ~inputsArr[i] ^ inputsArr[j]
            // In Go: !inputsArr[i] != inputsArr[j] gives the XOR
            equal := (!inputsArr[i]) != inputsArr[j]
            if equal {
                result |= 1 << bitPos
            }
            bitPos--
        }
    }

    out_out = result
}

func main() {}
