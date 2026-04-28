package main

var prev_clock bool
var out_p bool
var out_q bool

func TopModule(clock bool, a bool) {
    if clock {
        out_p = a
    } else {
        out_p = out_p
    }
    if prev_clock && !clock {
        out_q = a
    }
    prev_clock = clock
}

func main() {}
