package main

var state uint8
var out_fr2 bool
var out_fr1 bool
var out_fr0 bool
var out_dfr bool

func TopModule(clk bool, reset bool, s uint8) {
	const (
		A2 = uint8(0)
		B1 = uint8(1)
		B2 = uint8(2)
		C1 = uint8(3)
		C2 = uint8(4)
		D1 = uint8(5)
	)

	next := state
	s0 := (s & 0x1) != 0
	s1 := (s & 0x2) != 0
	s2 := (s & 0x4) != 0

	switch state {
	case A2:
		if s0 {
			next = B1
		} else {
			next = A2
		}
	case B1:
		if s1 {
			next = C1
		} else if s0 {
			next = B1
		} else {
			next = A2
		}
	case B2:
		if s1 {
			next = C1
		} else if s0 {
			next = B2
		} else {
			next = A2
		}
	case C1:
		if s2 {
			next = D1
		} else if s1 {
			next = C1
		} else {
			next = B2
		}
	case C2:
		if s2 {
			next = D1
		} else if s1 {
			next = C2
		} else {
			next = B2
		}
	case D1:
		if s2 {
			next = D1
		} else {
			next = C2
		}
	default:
		next = A2
	}

	if clk {
		if reset {
			state = A2
		} else {
			state = next
		}
	}

	switch state {
	case A2:
		out_fr2, out_fr1, out_fr0, out_dfr = true, true, true, true
	case B1:
		out_fr2, out_fr1, out_fr0, out_dfr = false, true, true, false
	case B2:
		out_fr2, out_fr1, out_fr0, out_dfr = false, true, true, true
	case C1:
		out_fr2, out_fr1, out_fr0, out_dfr = false, false, true, false
	case C2:
		out_fr2, out_fr1, out_fr0, out_dfr = false, false, true, true
	case D1:
		out_fr2, out_fr1, out_fr0, out_dfr = false, false, false, false
	default:
		out_fr2, out_fr1, out_fr0, out_dfr = false, false, false, false
	}
}

func main() {}
