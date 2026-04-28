package main

var out_sum [5]bool

func TopModule(x [4]bool, y [4]bool) {
    var carry bool = false
    var tempSum [5]bool

    // Full adder for bit 0
    tempSum[0] = (x[0] != y[0]) != carry
    carry = (x[0] && y[0]) || (x[0] && carry) || (y[0] && carry)

    // Full adder for bit 1
    tempSum[1] = (x[1] != y[1]) != carry
    carry = (x[1] && y[1]) || (x[1] && carry) || (y[1] && carry)

    // Full adder for bit 2
    tempSum[2] = (x[2] != y[2]) != carry
    carry = (x[2] && y[2]) || (x[2] && carry) || (y[2] && carry)

    // Full adder for bit 3
    tempSum[3] = (x[3] != y[3]) != carry
    tempSum[4] = carry

    out_sum = tempSum
}

func main() {}
