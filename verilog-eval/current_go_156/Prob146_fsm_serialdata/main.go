package main

var state_146 uint8 = 8
var byte_146 uint8
var out_byte uint8
var out_done bool

func TopModule(clk bool, in bool, reset bool) {
	next := state_146

	switch state_146 {
	case 8:
		if in {
			next = 8
		} else {
			next = 0
		}
	case 0:
		next = 1
	case 1:
		next = 2
	case 2:
		next = 3
	case 3:
		next = 4
	case 4:
		next = 5
	case 5:
		next = 6
	case 6:
		next = 7
	case 7:
		next = 9
	case 9:
		if in {
			next = 10
		} else {
			next = 11
		}
	case 10:
		if in {
			next = 8
		} else {
			next = 0
		}
	case 11:
		if in {
			next = 8
		} else {
			next = 11
		}
	}

	if clk {
		oldState := state_146
		oldByte := byte_146
		if reset {
			state_146 = 8
		} else {
			state_146 = next
		}

		switch oldState {
		case 0:
			if in {
				byte_146 = oldByte | 0x01
			} else {
				byte_146 = oldByte & 0xfe
			}
		case 1:
			if in {
				byte_146 = oldByte | 0x02
			} else {
				byte_146 = oldByte & 0xfd
			}
		case 2:
			if in {
				byte_146 = oldByte | 0x04
			} else {
				byte_146 = oldByte & 0xfb
			}
		case 3:
			if in {
				byte_146 = oldByte | 0x08
			} else {
				byte_146 = oldByte & 0xf7
			}
		case 4:
			if in {
				byte_146 = oldByte | 0x10
			} else {
				byte_146 = oldByte & 0xef
			}
		case 5:
			if in {
				byte_146 = oldByte | 0x20
			} else {
				byte_146 = oldByte & 0xdf
			}
		case 6:
			if in {
				byte_146 = oldByte | 0x40
			} else {
				byte_146 = oldByte & 0xbf
			}
		case 7:
			if in {
				byte_146 = oldByte | 0x80
			} else {
				byte_146 = oldByte & 0x7f
			}
		}
	}

	out_done = state_146 == 10
	out_byte = byte_146
}

func main() {}
