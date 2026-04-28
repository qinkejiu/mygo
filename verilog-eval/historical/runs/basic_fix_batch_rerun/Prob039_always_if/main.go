package main

var out_assign bool
var out_always bool

func TopModule(a bool, b bool, sel_b1 bool, sel_b2 bool) {
    // Combinational logic using assign-style expression
    if sel_b1 && sel_b2 {
        out_assign = b
    } else {
        out_assign = a
    }
    
    // Combinational logic using procedural if statement
    if sel_b1 && sel_b2 {
        out_always = b
    } else {
        out_always = a
    }
}

func main() {}
