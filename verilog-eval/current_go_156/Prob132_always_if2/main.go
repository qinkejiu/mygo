package main

var out_shut_off_computer bool
var out_keep_driving bool

func TopModule(cpu_overheated bool, arrived bool, gas_tank_empty bool) {
    if cpu_overheated {
        out_shut_off_computer = true
    } else {
        out_shut_off_computer = false
    }

    if !arrived {
        out_keep_driving = !gas_tank_empty
    } else {
        out_keep_driving = false
    }
}

func main() {}
