package main

var out_q uint16

func TopModule(clk bool, resetn bool, byteena uint8, d uint16) {
    // Static variable to hold the register state
    var reg uint16

    // On positive edge of clk
    if clk {
        if !resetn {
            reg = 0
        } else {
            // Process lower byte (byteena[0])
            if (byteena & 0x01) != 0 {
                reg = (reg & 0xFF00) | (d & 0x00FF)
            }
            // Process upper byte (byteena[1])
            if (byteena & 0x02) != 0 {
                reg = (reg & 0x00FF) | (d & 0xFF00)
            }
        }
    }

    // Output the register value
    out_q = reg
}

func main() {}
