package main

var out_both uint8
var out_any uint8
var out_different uint8

func TopModule(in uint8) {
	out_both = (in & 0x7) & ((in >> 1) & 0x7)
	out_any = ((in & 0x7) | ((in >> 1) & 0x7)) << 1
	out_different = in ^ (((in & 0x1) << 3) | ((in >> 1) & 0x7))
}

func main() {}
