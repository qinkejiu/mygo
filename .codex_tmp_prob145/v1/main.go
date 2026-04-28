package main

var prev_clock bool
var out_p bool
var out_q bool

func TopModule(clock bool, a bool) {
    nextP := out_p
    if clock {
        nextP = a
    }
    out_p = nextP
    if prev_clock && !clock {
        out_q = a
    }
    prev_clock = clock
}

func main() {}
