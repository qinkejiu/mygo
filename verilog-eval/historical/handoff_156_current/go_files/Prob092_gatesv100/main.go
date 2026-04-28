package main

var out_both [100]bool
var out_any [100]bool
var out_different [100]bool

func TopModule(in [100]bool) {
	var both [100]bool
	var any [100]bool
	var different [100]bool

	both[99] = false
	for i := 0; i < 99; i++ {
		both[i] = false
		if in[i] {
			if in[i+1] {
				both[i] = true
			}
		}
	}

	any[0] = false
	for i := 1; i < 100; i++ {
		any[i] = false
		if in[i] {
			any[i] = true
		}
		if in[i-1] {
			any[i] = true
		}
	}

	for i := 0; i < 99; i++ {
		different[i] = in[i]
		if in[i+1] {
			different[i] = !different[i]
		}
	}
	different[99] = in[99]
	if in[0] {
		different[99] = !different[99]
	}

	for i := 0; i < 100; i++ {
		out_both[i] = both[i]
		out_any[i] = any[i]
		out_different[i] = different[i]
	}
}

func main() {}
