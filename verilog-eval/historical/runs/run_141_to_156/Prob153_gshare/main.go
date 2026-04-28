package main

var out_predict_taken bool
var out_predict_history uint8

func TopModule(
    clk bool,
    areset bool,
    predict_valid bool,
    predict_pc uint8,
    train_valid bool,
    train_taken bool,
    train_mispredicted bool,
    train_history uint8,
    train_pc uint8,
) {
    // PHT: 128 entries of 2-bit saturating counters
    // Use uint8 array where bits [1:0] are the counter
    var pht [128]uint8
    
    // 7-bit global history register
    var predict_history_r uint8
    
    // 7-bit indices (only need 7 bits, but using uint8 for convenience)
    var predict_index uint8
    var train_index uint8
    
    // Initialize on reset
    if areset {
        // Initialize all PHT entries to LNT (01)
        for i := 0; i < 128; i++ {
            pht[i] = 1 // LNT = 01
        }
        predict_history_r = 0
    } else {
        // Calculate indices
        predict_index = (predict_history_r ^ predict_pc) & 0x7F
        train_index = (train_history ^ train_pc) & 0x7F
        
        // Update PHT on training
        if train_valid {
            current_counter := pht[train_index] & 0x03
            
            // Update counter based on actual outcome
            if train_taken {
                if current_counter < 3 {
                    pht[train_index] = (current_counter + 1) & 0x03
                }
            } else {
                if current_counter > 0 {
                    pht[train_index] = (current_counter - 1) & 0x03
                }
            }
            
            // Update history register on misprediction
            if train_mispredicted {
                // {train_history, train_taken}
                predict_history_r = ((train_history << 1) & 0x7F)
                if train_taken {
                    predict_history_r |= 0x01
                } else {
                    predict_history_r &= 0xFE
                }
            }
        }
        
        // Update history register on prediction (if no training misprediction)
        if predict_valid && (!train_valid || !train_mispredicted) {
            // Get prediction for history update
            counter_val := pht[predict_index] & 0x03
            predicted_taken := (counter_val >> 1) & 0x01
            
            // {predict_history_r, predicted_taken}
            predict_history_r = ((predict_history_r << 1) & 0x7F)
            if predicted_taken {
                predict_history_r |= 0x01
            } else {
                predict_history_r &= 0xFE
            }
        }
    }
    
    // Calculate outputs
    if predict_valid {
        // Recalculate predict_index for output (uses current predict_history_r)
        predict_index = (predict_history_r ^ predict_pc) & 0x7F
        counter_val := pht[predict_index] & 0x03
        out_predict_taken = (counter_val>>1)&0x01 != 0
        out_predict_history = predict_history_r & 0x7F
    } else {
        out_predict_taken = false
        out_predict_history = 0
    }
}

func main() {}
