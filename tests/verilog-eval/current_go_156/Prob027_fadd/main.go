package main

var out_cout bool
var out_sum bool

func TopModule(a bool, b bool, cin bool) {
	out_sum = (a != b) != cin

	out_cout = false
	if a {
		if b {
			out_cout = true
		}
		if cin {
			out_cout = true
		}
	}
	if b {
		if cin {
			out_cout = true
		}
	}
}

func main() {}
