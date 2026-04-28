package main

var out_sum uint8

func TopModule(x uint8, y uint8) {
    // 4-bit adder with carry chain
    // Extract 4-bit inputs (ignore higher bits)
    x4 := x & 0x0F
    y4 := y & 0x0F
    
    // Full adder implementation
    // Bit 0
    sum0 := (x4 & 0x01) ^ (y4 & 0x01)
    carry0 := (x4 & 0x01) & (y4 & 0x01)
    
    // Bit 1
    x1 := (x4 >> 1) & 0x01
    y1 := (y4 >> 1) & 0x01
    sum1 := x1 ^ y1 ^ carry0
    carry1 := (x1 & y1) | (x1 & carry0) | (y1 & carry0)
    
    // Bit 2
    x2 := (x4 >> 2) & 0x01
    y2 := (y4 >> 2) & 0x01
    sum2 := x2 ^ y2 ^ carry1
    carry2 := (x2 & y2) | (x2 & carry1) | (y2 & carry1)
    
    // Bit 3
    x3 := (x4 >> 3) & 0x01
    y3 := (y4 >> 3) & 0x01
    sum3 := x3 ^ y3 ^ carry2
    carry3 := (x3 & y3) | (x3 & carry2) | (y3 & carry2)
    
    // Combine results: 5-bit sum with overflow bit
    out_sum = (uint8(carry3) << 4) | (sum3 << 3) | (sum2 << 2) | (sum1 << 1) | sum0
}

func main() {}
