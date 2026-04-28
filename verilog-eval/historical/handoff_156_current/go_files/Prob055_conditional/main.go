package main

var out_min uint8

func TopModule(a uint8, b uint8, c uint8, d uint8) {
	min := a
	if b < min {
		min = b
	}
	if c < min {
		min = c
	}
	if d < min {
		min = d
	}
	out_min = min
}

func main() {}
