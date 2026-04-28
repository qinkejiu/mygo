package main

var out_assign bool
var out_alwaysblock bool

func TopModule(a bool, b bool) {
    // assign statement equivalent
    out_assign = a && b
    
    // combinational always block equivalent
    out_alwaysblock = a && b
}

func main() {}
