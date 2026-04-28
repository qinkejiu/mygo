package main

var out_out uint32

func TopModule(a bool, b bool, c bool, d bool, e bool) {
	var bits uint32
	if a {
		bits |= 1 << 4
	}
	if b {
		bits |= 1 << 3
	}
	if c {
		bits |= 1 << 2
	}
	if d {
		bits |= 1 << 1
	}
	if e {
		bits |= 1 << 0
	}

	var firstVec uint32
	if a {
		firstVec |= 0x1F << 20
	}
	if b {
		firstVec |= 0x1F << 15
	}
	if c {
		firstVec |= 0x1F << 10
	}
	if d {
		firstVec |= 0x1F << 5
	}
	if e {
		firstVec |= 0x1F
	}

	secondVec := (bits << 20) | (bits << 15) | (bits << 10) | (bits << 5) | bits
	out_out = ((^firstVec) & 0x1FFFFFF) ^ secondVec
}

func main() {}
