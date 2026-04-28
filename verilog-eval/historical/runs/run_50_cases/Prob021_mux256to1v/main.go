package main

var out_out uint8

func TopModule(in [1024]bool, sel uint8) {
    // Calculate the starting bit position for the selected 4-bit group
    start := uint16(sel) * 4
    
    // Extract the 4 bits using array indexing
    // Note: in[0] is LSB, in[1023] is MSB in our array representation
    // But the Verilog reference uses in[sel*4+0] as LSB, in[sel*4+3] as MSB
    // So we'll follow the same convention: bit 0 is LSB of the selected group
    
    // Build the 4-bit value from the selected bits
    var result uint8 = 0
    if in[start] {
        result |= 0x1
    }
    if in[start+1] {
        result |= 0x2
    }
    if in[start+2] {
        result |= 0x4
    }
    if in[start+3] {
        result |= 0x8
    }
    
    out_out = result
}

func main() {}
