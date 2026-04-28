package main

var out_assign bool
var out_always bool

func TopModule(a bool, b bool, sel_b1 bool, sel_b2 bool) {
    // assign version
    out_assign = (sel_b1 && sel_b2) ? b : a
    
    // procedural if version
    if sel_b1 && sel_b2 {
        out_always = b
    } else {
        out_always = a
    }
}

func main() {}
