package main

var out_heater bool
var out_aircon bool
var out_fan bool

func TopModule(mode bool, too_cold bool, too_hot bool, fan_on bool) {
    // Heater logic: only on in heating mode (mode=true) when too_cold
    out_heater = mode && too_cold
    
    // Air conditioner logic: only on in cooling mode (mode=false) when too_hot
    out_aircon = !mode && too_hot
    
    // Fan logic: on when heater or aircon is on, OR when explicitly requested
    out_fan = out_heater || out_aircon || fan_on
}

func main() {}
