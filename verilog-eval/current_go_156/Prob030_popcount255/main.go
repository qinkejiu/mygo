package main

var out_out uint8

func TopModule(in [255]bool) {
    count := uint8(0)
    
    for i := 0; i < 255; i++ {
        if in[i] {
            count++
        }
    }
    
    out_out = count
}

func main() {}
