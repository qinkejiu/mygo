package main

var out_parity bool

func TopModule(in uint8) {
    // Compute even parity: XOR of all 8 bits
    // For uint8, we can compute parity by repeatedly XORing shifted values
    // or use a direct XOR chain
    p := ((in >> 0) & 1) ^
         ((in >> 1) & 1) ^
         ((in >> 2) & 1) ^
         ((in >> 3) & 1) ^
         ((in >> 4) & 1) ^
         ((in >> 5) & 1) ^
         ((in >> 6) & 1) ^
         ((in >> 7) & 1)
    
    out_parity = p != 0
}

func main() {}
