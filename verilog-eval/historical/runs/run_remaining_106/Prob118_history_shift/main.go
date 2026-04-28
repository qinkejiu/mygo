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
    // Sequential logic triggered on positive edge of clk
    // We'll model this with a simple state variable
    var history uint32

    // Reset condition (positive edge triggered asynchronous reset)
    if areset {
        history = 0
    } else {
        // Check for misprediction first (highest priority)
        if train_mispredicted {
            // Shift train_history left by 1 and insert train_taken at LSB
            history = (train_history << 1) | uint32(train_taken&0x1)
        } else if predict_valid {
            // Shift current history left by 1 and insert predict_taken at LSB
            history = (history << 1) | uint32(predict_taken&0x1)
        }
        // If neither condition, history retains its value
    }

    // Assign to output global
    out_predict_history = history
}

func main() {}
