package main

var out_ringer bool
var out_motor bool

func TopModule(ring bool, vibrate_mode bool) {
    out_ringer = ring && !vibrate_mode
    out_motor = ring && vibrate_mode
}

func main() {}
