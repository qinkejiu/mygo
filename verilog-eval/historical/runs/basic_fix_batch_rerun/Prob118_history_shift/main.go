package main

var out_predict_history uint32

func TopModule(
    clk bool,
    areset bool,
    predict_valid bool,
    predict_taken bool,
    train_mispredicted bool,
    train_taken bool,
    train_history uint32,
) {
    // Internal state variable
    var history uint32

    // Sequential logic triggered on positive edge of clk
    if clk {
        if areset {
            history = 0
        } else {
            if train_mispredicted {
                // Rollback: train_history concatenated with train_taken
                var taken_bit uint32 = 0
                if train_taken {
                    taken_bit = 1
                }
                history = (train_history << 1) | taken_bit
            } else if predict_valid {
                // Normal update: shift in predict_taken from LSB side
                var taken_bit uint32 = 0
                if predict_taken {
                    taken_bit = 1
                }
                history = (history << 1) | taken_bit
            }
        }
        // Update output
        out_predict_history = history
    }
}

func main() {}
