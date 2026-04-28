package main

var out_out uint8
var out_result_is_zero bool

func TopModule(do_sub bool, a uint8, b uint8) {
    var result uint8
    
    if !do_sub {
        result = a + b
    } else {
        result = a - b
    }
    
    out_out = result
    out_result_is_zero = (result == 0)
}

func main() {}
