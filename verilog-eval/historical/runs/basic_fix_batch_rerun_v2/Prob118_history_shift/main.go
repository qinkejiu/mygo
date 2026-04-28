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
    if areset {
        out_predict_history = 0
    } else if clk {
        if train_mispredicted {
            out_predict_history = (train_history << 1) | uint32(b2i(train_taken))
        } else if predict_valid {
            out_predict_history = (out_predict_history << 1) | uint32(b2i(predict_taken))
        }
    }
}

func b2i(b bool) uint32 {
    if b {
        return 1
    }
    return 0
}

func main() {}
