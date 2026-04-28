package main

var out_Y1 bool
var out_Y3 bool

func TopModule(y uint8, w bool) {
	out_Y1 = (y&0x01) != 0 && !w
	out_Y3 = (y&0x36) != 0 && w
}

func main() {}
