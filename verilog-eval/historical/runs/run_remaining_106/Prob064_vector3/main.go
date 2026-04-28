package main

var out_w uint8
var out_x uint8
var out_y uint8
var out_z uint8

func TopModule(a uint8, b uint8, c uint8, d uint8, e uint8, f uint8) {
    // Concatenate all inputs into a 30-bit value with two 1 bits appended at LSB
    // Since each input is 5 bits, we need to mask them to 5 bits first
    a5 := a & 0x1F
    b5 := b & 0x1F
    c5 := c & 0x1F
    d5 := d & 0x1F
    e5 := e & 0x1F
    f5 := f & 0x1F
    
    // Build the 32-bit concatenated value: {a,b,c,d,e,f,2'b11}
    // Position: a[4:0] at bits [31:27], b[4:0] at bits [26:22], ..., f[4:0] at bits [7:3], 2'b11 at bits [2:1]
    concat := (uint32(a5) << 27) |
              (uint32(b5) << 22) |
              (uint32(c5) << 17) |
              (uint32(d5) << 12) |
              (uint32(e5) << 7) |
              (uint32(f5) << 2) |
              0x3 // 2'b11
    
    // Split into four 8-bit outputs
    // w gets bits [31:24], x gets bits [23:16], y gets bits [15:8], z gets bits [7:0]
    out_w = uint8((concat >> 24) & 0xFF)
    out_x = uint8((concat >> 16) & 0xFF)
    out_y = uint8((concat >> 8) & 0xFF)
    out_z = uint8(concat & 0xFF)
}

func main() {}
