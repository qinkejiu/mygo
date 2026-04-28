package main

var out_cout bool
var out_sum bool

func TopModule(a bool, b bool, cin bool) {
    // Full adder logic
    // sum = a xor b xor cin
    out_sum = (a != b) != cin
    
    // cout = (a & b) | (a & cin) | (b & cin)
    out_cout = (a && b) || (a && cin) || (b && cin)
}

func main() {}
