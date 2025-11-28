package main

import "fmt"

func main() {
	var i, dead, l int32
	var j int16
	var k int64

	dead = 3
	l = dead
	i = 1
	j = 2
	k = int64(i) + int64(j)
	_ = l

	fmt.Printf("The result is small: %d\n", k)
}
