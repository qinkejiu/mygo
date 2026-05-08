package main

var state_156 uint8
var scount_156 uint8
var fcount_156 uint16
var out_count uint8
var out_counting bool
var out_done bool

func TopModule(clk bool, reset bool, data bool, ack bool) {
	counting := state_156 == 8
	doneCounting := (scount_156&0x0f) == 0 && fcount_156 == 999
	next := state_156

	switch state_156 {
	case 0:
		if data {
			next = 1
		} else {
			next = 0
		}
	case 1:
		if data {
			next = 2
		} else {
			next = 0
		}
	case 2:
		if data {
			next = 2
		} else {
			next = 3
		}
	case 3:
		if data {
			next = 4
		} else {
			next = 0
		}
	case 4:
		next = 5
	case 5:
		next = 6
	case 6:
		next = 7
	case 7:
		next = 8
	case 8:
		if doneCounting {
			next = 9
		} else {
			next = 8
		}
	case 9:
		if ack {
			next = 0
		} else {
			next = 9
		}
	}

	if clk {
		oldState := state_156
		oldScount := scount_156
		oldFcount := fcount_156
		if reset {
			state_156 = 0
		} else {
			state_156 = next
		}

		switch oldState {
		case 4:
			if data {
				scount_156 = (oldScount | 0x08) & 0x0f
			} else {
				scount_156 = oldScount & 0x07
			}
		case 5:
			if data {
				scount_156 = (oldScount | 0x04) & 0x0f
			} else {
				scount_156 = oldScount & 0x0b
			}
		case 6:
			if data {
				scount_156 = (oldScount | 0x02) & 0x0f
			} else {
				scount_156 = oldScount & 0x0d
			}
		case 7:
			if data {
				scount_156 = (oldScount | 0x01) & 0x0f
			} else {
				scount_156 = oldScount & 0x0e
			}
		default:
			if counting && oldFcount == 999 {
				scount_156 = (oldScount - 1) & 0x0f
			}
		}

		if !counting || oldFcount == 999 {
			fcount_156 = 0
		} else {
			fcount_156 = oldFcount + 1
		}
	}

	out_counting = state_156 == 8
	out_done = state_156 == 9
	out_count = scount_156 & 0x0f
}

func main() {}
