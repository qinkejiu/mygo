package main

var out_out [100]bool

func TopModule(in [100]bool) {
    for i := 0; i < 100; i++ {
        out_out[i] = in[99-i]
    }
}

func main() {}
