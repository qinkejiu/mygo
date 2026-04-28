package main

var out_and bool
var out_or bool
var out_xor bool

func TopModule(in [100]bool) {
	andResult := true
	orResult := false
	xorResult := false

	for i := 0; i < 100; i++ {
		if !in[i] {
			andResult = false
		}
		if in[i] {
			orResult = true
			xorResult = !xorResult
		}
	}

	out_and = andResult
	out_or = orResult
	out_xor = xorResult
}

func main() {}
