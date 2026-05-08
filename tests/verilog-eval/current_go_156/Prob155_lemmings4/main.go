package main

const (
	wl_155    = 0
	wr_155    = 1
	falll_155 = 2
	fallr_155 = 3
	digl_155  = 4
	digr_155  = 5
	dead_155  = 6
)

var prev_clk_155 bool
var state_155 uint8
var fall_counter_155 uint8
var out_walk_left bool
var out_walk_right bool
var out_aaah bool
var out_digging bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool, ground bool, dig bool) {
	oldState := state_155
	oldFallCounter := fall_counter_155
	nextState := oldState

	switch oldState {
	case wl_155:
		if !ground {
			nextState = falll_155
		} else if dig {
			nextState = digl_155
		} else if bump_left {
			nextState = wr_155
		}
	case wr_155:
		if !ground {
			nextState = fallr_155
		} else if dig {
			nextState = digr_155
		} else if bump_right {
			nextState = wl_155
		}
	case falll_155:
		if ground {
			if oldFallCounter >= 20 {
				nextState = dead_155
			} else {
				nextState = wl_155
			}
		}
	case fallr_155:
		if ground {
			if oldFallCounter >= 20 {
				nextState = dead_155
			} else {
				nextState = wr_155
			}
		}
	case digl_155:
		if !ground {
			nextState = falll_155
		}
	case digr_155:
		if !ground {
			nextState = fallr_155
		}
	case dead_155:
		nextState = dead_155
	}

	if areset {
		state_155 = wl_155
		fall_counter_155 = 0
	} else if !prev_clk_155 && clk {
		state_155 = nextState
		if oldState == falll_155 || oldState == fallr_155 {
			if oldFallCounter < 20 {
				fall_counter_155 = oldFallCounter + 1
			}
		} else {
			fall_counter_155 = 0
		}
	}

	prev_clk_155 = clk

	out_walk_left = state_155 == wl_155
	out_walk_right = state_155 == wr_155
	out_aaah = state_155 == falll_155 || state_155 == fallr_155
	out_digging = state_155 == digl_155 || state_155 == digr_155
}

func main() {}
