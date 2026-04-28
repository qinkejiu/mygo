package main

var out_p1y bool
var out_p2y bool

func TopModule(p1a bool, p1b bool, p1c bool, p1d bool, p1e bool, p1f bool, p2a bool, p2b bool, p2c bool, p2d bool) {
	out_p1y = false
	if p1a {
		if p1b {
			if p1c {
				out_p1y = true
			}
		}
	}
	if p1d {
		if p1e {
			if p1f {
				out_p1y = true
			}
		}
	}

	out_p2y = false
	if p2a {
		if p2b {
			out_p2y = true
		}
	}
	if p2c {
		if p2d {
			out_p2y = true
		}
	}
}

func main() {}
