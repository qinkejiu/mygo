package main

var out_q [256]bool

func TopModule(clk bool, load bool, data [256]bool) {
    // Sequential logic triggered on positive clock edge
    if clk {
        if load {
            // Load data into q
            for i := 0; i < 256; i++ {
                out_q[i] = data[i]
            }
        } else {
            // Calculate next state based on current out_q
            var next_q [256]bool
            
            // Create padded 18x18 grid for neighbor counting with wrap-around
            var padded [324]bool // 18*18 = 324
            
            // Copy 16x16 grid to center of padded grid (rows 1-16, cols 1-16)
            for i := 0; i < 16; i++ {
                for j := 0; j < 16; j++ {
                    padded[(i+1)*18 + (j+1)] = out_q[i*16 + j]
                }
            }
            
            // Wrap-around: copy bottom row to top padding (row 0)
            for j := 0; j < 16; j++ {
                padded[1 + j] = out_q[15*16 + j] // row 15
            }
            
            // Wrap-around: copy top row to bottom padding (row 17)
            for j := 0; j < 16; j++ {
                padded[17*18 + 1 + j] = out_q[0*16 + j] // row 0
            }
            
            // Wrap-around: copy right column to left padding (col 0)
            for i := 0; i < 18; i++ {
                padded[i*18] = padded[i*18 + 16]
            }
            
            // Wrap-around: copy left column to right padding (col 17)
            for i := 0; i < 18; i++ {
                padded[i*18 + 17] = padded[i*18 + 1]
            }
            
            // Calculate next state for each cell
            for i := 0; i < 16; i++ {
                for j := 0; j < 16; j++ {
                    // Count neighbors in 3x3 neighborhood (excluding center)
                    centerRow := i + 1
                    centerCol := j + 1
                    
                    // Get neighbor positions relative to center
                    neighborPositions := [8][2]int{
                        {-1, -1}, {-1, 0}, {-1, 1},
                        {0, -1},           {0, 1},
                        {1, -1},  {1, 0},  {1, 1},
                    }
                    
                    // Count live neighbors
                    liveNeighbors := 0
                    for _, pos := range neighborPositions {
                        row := centerRow + pos[0]
                        col := centerCol + pos[1]
                        if padded[row*18 + col] {
                            liveNeighbors++
                        }
                    }
                    
                    // Apply game rules
                    currentState := out_q[i*16 + j]
                    switch {
                    case liveNeighbors <= 1:
                        next_q[i*16 + j] = false
                    case liveNeighbors == 2:
                        next_q[i*16 + j] = currentState
                    case liveNeighbors == 3:
                        next_q[i*16 + j] = true
                    default: // 4 or more neighbors
                        next_q[i*16 + j] = false
                    }
                }
            }
            
            // Update output
            for i := 0; i < 256; i++ {
                out_q[i] = next_q[i]
            }
        }
    }
}

func main() {}
