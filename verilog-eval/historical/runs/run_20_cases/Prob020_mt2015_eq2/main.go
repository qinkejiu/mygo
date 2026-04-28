package main

var out_z bool

func TopModule(A [2]bool, B [2]bool) {
    out_z = (A[0] == B[0]) && (A[1] == B[1])
}

func main() {}
