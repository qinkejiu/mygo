package main

var prev_clock bool
var out_p bool
var out_q bool

func TopModule(clock bool, a bool) {
    tmp := a
    if !clock {
        tmp = out_p
    }
    out_p = tmp
    if prev_clock && !clock {
        out_q = a
    }
    prev_clock = clock
}

func main() {}
