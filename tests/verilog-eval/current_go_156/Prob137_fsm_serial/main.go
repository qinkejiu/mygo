package main

var out_done bool

func TopModule(clk bool, reset bool, in bool) {
    // State encoding constants
    const (
        STATE_START = 0
        STATE_B0    = 1
        STATE_B1    = 2
        STATE_B2    = 3
        STATE_B3    = 4
        STATE_B4    = 5
        STATE_B5    = 6
        STATE_B6    = 7
        STATE_B7    = 8
        STATE_STOP  = 9
        STATE_DONE  = 10
        STATE_ERR   = 11
    )

    // State register
    var state uint8 = STATE_START
    var next_state uint8

    // Combinational next state logic
    switch state {
    case STATE_START:
        if in {
            next_state = STATE_START
        } else {
            next_state = STATE_B0
        }
    case STATE_B0:
        next_state = STATE_B1
    case STATE_B1:
        next_state = STATE_B2
    case STATE_B2:
        next_state = STATE_B3
    case STATE_B3:
        next_state = STATE_B4
    case STATE_B4:
        next_state = STATE_B5
    case STATE_B5:
        next_state = STATE_B6
    case STATE_B6:
        next_state = STATE_B7
    case STATE_B7:
        next_state = STATE_STOP
    case STATE_STOP:
        if in {
            next_state = STATE_DONE
        } else {
            next_state = STATE_ERR
        }
    case STATE_DONE:
        if in {
            next_state = STATE_START
        } else {
            next_state = STATE_B0
        }
    case STATE_ERR:
        if in {
            next_state = STATE_START
        } else {
            next_state = STATE_ERR
        }
    default:
        next_state = STATE_START
    }

    // Sequential logic triggered on positive edge of clk
    if clk {
        if reset {
            state = STATE_START
        } else {
            state = next_state
        }
    }

    // Output logic
    out_done = (state == STATE_DONE)
}

func main() {}
