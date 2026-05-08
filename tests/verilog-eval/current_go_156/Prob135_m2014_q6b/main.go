package main

var out_Y1 bool

func TopModule(y uint8, w bool) {
	out_Y1 = false
	switch ((y & 0x7) << 1) | b2u(w) {
	case 0x2, 0x3, 0x5, 0x9, 0xA, 0xB:
		out_Y1 = true
	}
}

func b2u(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

func main() {}
