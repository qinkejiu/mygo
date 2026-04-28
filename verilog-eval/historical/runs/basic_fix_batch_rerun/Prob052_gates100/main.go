package main

var out_and bool
var out_or bool
var out_xor bool

func TopModule(in [100]bool) {
    andResult := true
    orResult := false
    xorResult := false
    
    for i := 0; i < 100; i++ {
        bit := in[i]
        andResult = andResult && bit
        orResult = orResult || bit
        if bit {
            xorResult = !xorResult
        }
    }
    
    out_and = andResult
    out_or = orResult
    out_xor = xorResult
}

func main() {}
