package main

var out_count uint8
var out_counting bool
var out_done bool

func TopModule(clk bool, reset bool, data bool, ack bool) {
    // State encoding
    const (
        S = iota
        S1
        S11
        S110
        B0
        B1
        B2
        B3
        Count
        Wait
    )

    // State register
    var state uint8
    var nextState uint8

    // Shift register for delay value
    var scount uint8

    // Fast counter (0-999)
    var fcount uint16

    // Determine next state
    nextState = state
    switch state {
    case S:
        if data {
            nextState = S1
        } else {
            nextState = S
        }
    case S1:
        if data {
            nextState = S11
        } else {
            nextState = S
        }
    case S11:
        if data {
            nextState = S11
        } else {
            nextState = S110
        }
    case S110:
        if data {
            nextState = B0
        } else {
            nextState = S
        }
    case B0:
        nextState = B1
    case B1:
        nextState = B2
    case B2:
        nextState = B3
    case B3:
        nextState = Count
    case Count:
        // Check if counting is done
        doneCounting := (scount == 0) && (fcount == 999)
        if doneCounting {
            nextState = Wait
        } else {
            nextState = Count
        }
    case Wait:
        if ack {
            nextState = S
        } else {
            nextState = Wait
        }
    }

    // Clocked logic
    if clk {
        if reset {
            state = S
            scount = 0
            fcount = 0
        } else {
            state = nextState

            // Shift register update
            if state == B0 || state == B1 || state == B2 || state == B3 {
                // Shift in data (MSB first)
                scount = (scount << 1) & 0x0F
                if data {
                    scount |= 0x01
                }
            } else if state == Count && fcount == 999 {
                // Decrement scount every 1000 cycles
                if scount > 0 {
                    scount -= 1
                }
            }

            // Fast counter update
            if state == Count {
                if fcount == 999 {
                    fcount = 0
                } else {
                    fcount += 1
                }
            } else {
                fcount = 0
            }
        }
    }

    // Output assignments
    out_counting = (state == Count)
    out_done = (state == Wait)

    // Count output - only valid when counting
    if state == Count {
        out_count = scount
    } else {
        // Don't-care value (convenient to implement as 0)
        out_count = 0
    }
}

func main() {}
