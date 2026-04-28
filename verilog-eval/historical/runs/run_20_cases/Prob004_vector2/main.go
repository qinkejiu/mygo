package main

var out_out [32]bool

func TopModule(in [32]bool) {
    // Reverse byte order: {in[7:0], in[15:8], in[23:16], in[31:24]}
    
    // Byte 0 (bits 7:0) becomes bits 31:24
    for i := 0; i < 8; i++ {
        out_out[31-i] = in[7-i]
    }
    
    // Byte 1 (bits 15:8) becomes bits 23:16
    for i := 0; i < 8; i++ {
        out_out[23-i] = in[15-i]
    }
    
    // Byte 2 (bits 23:16) becomes bits 15:8
    for i := 0; i < 8; i++ {
        out_out[15-i] = in[23-i]
    }
    
    // Byte 3 (bits 31:24) becomes bits 7:0
    for i := 0; i < 8; i++ {
        out_out[7-i] = in[31-i]
    }
}

func main() {}
