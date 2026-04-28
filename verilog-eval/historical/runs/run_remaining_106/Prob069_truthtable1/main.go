package main

var out_f bool

func TopModule(x3 bool, x2 bool, x1 bool) {
    out_f = (!x3 && x2 && !x1) ||
            (!x3 && x2 && x1) ||
            (x3 && !x2 && x1) ||
            (x3 && x2 && x1)
}

func main() {}
