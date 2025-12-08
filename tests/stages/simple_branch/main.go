package main

import "fmt"

func sink(v int32) {}

func main() {
	var x, y, delta int32
	x = 10
	y = 3
	if x > y {
		delta = x - y
		fmt.Printf("branch x>y delta=%d (x=%d y=%d)\n", delta, x, y)
	} else {
		delta = y - x
		fmt.Printf("branch y>=x delta=%d (x=%d y=%d)\n", delta, x, y)
	}
	sink(delta)
}
