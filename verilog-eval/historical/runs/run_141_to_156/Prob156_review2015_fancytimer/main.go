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

    // State registers
    var state uint8
    var next uint8

    // Shift register for delay[3:0]
    var scount uint8
    // Fast counter for 0-999 cycles
    var fcount uint16

    // Control signals
    var shift_ena bool
    var counting_local bool
    var done_local bool

    // Determine next state
    switch state {
    case S:
        if data {
            next = S1
        } else {
            next = S
        }
    case S1:
        if data {
            next = S11
        } else {
            next = S
        }
    case S11:
        if data {
            next = S11
        } else {
            next = S110
        }
    case S110:
        if data {
            next = B0
        } else {
            next = S
        }
    case B0:
        next = B1
    case B1:
        next = B2
    case B2:
        next = B3
    case B3:
        next = Count
    case Count:
        if (scount == 0) && (fcount == 999) {
            next = Wait
        } else {
            next = Count
        }
    case Wait:
        if ack {
            next = S
        } else {
            next = Wait
        }
    default:
        next = S
    }

    // State transition on clock edge
    if clk {
        if reset {
            state = S
            scount = 0
            fcount = 0
        } else {
            state = next
        }
    }

    // Control logic
    shift_ena = (state == B0) || (state == B1) || (state == B2) || (state == B3)
    counting_local = (state == Count)
    done_local = (state == Wait)

    // Shift register update
    if clk {
        if shift_ena {
            // Shift in data MSB first
            scount = (scount << 1) & 0x0F
            if data {
                scount = scount | 0x01
            }
        } else if counting_local && (fcount == 999) {
            // Decrement scount every 1000 cycles
            if scount > 0 {
                scount = scount - 1
            }
        }
    }

    // Fast counter update
    if clk {
        if !counting_local {
            fcount = 0
        } else if fcount == 999 {
            fcount = 0
        } else {
            fcount = fcount + 1
        }
    }

    // Output assignments
    if counting_local {
        out_count = scount
    } else {
        out_count = 0 // Don't-care value, using 0 for convenience
    }
    out_counting = counting_local
    out_done = done_local
}

func main() {}
