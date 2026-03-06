package main

var input [8]int
var result int

func main() {

    i := 0
    max := input[0]

    for i < 8 {
        if input[i] > max {
            max = input[i]
        }
        i++
    }

    result = max
}
