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
            // Compute next state using Conway's Game of Life rules
            var next_q [256]bool
            
            // Create padded 18x18 grid for toroidal wrapping
            var padded [324]bool // 18*18 = 324
            
            // Copy q into center 16x16 region of padded grid
            for i := 0; i < 16; i++ {
                for j := 0; j < 16; j++ {
                    padded[(i+1)*18 + (j+1)] = out_q[i*16 + j]
                }
            }
            
            // Wrap top and bottom rows
            for j := 0; j < 16; j++ {
                padded[1*18 + (j+1)] = out_q[15*16 + j]      // top border gets bottom row
                padded[17*18 + (j+1)] = out_q[0*16 + j]      // bottom border gets top row
            }
            
            // Wrap left and right columns
            for i := 0; i < 18; i++ {
                padded[i*18 + 0] = padded[i*18 + 16]         // left border gets right column
                padded[i*18 + 17] = padded[i*18 + 1]         // right border gets left column
            }
            
            // Compute next state for each cell
            for i := 0; i < 16; i++ {
                for j := 0; j < 16; j++ {
                    // Count neighbors in 3x3 region centered at (i+1, j+1) in padded grid
                    centerRow := i + 1
                    centerCol := j + 1
                    baseIndex := centerRow*18 + centerCol
                    
                    // Count live neighbors (excluding center cell)
                    count := 0
                    
                    // Top row neighbors
                    if padded[baseIndex - 1 - 18] { count++ }
                    if padded[baseIndex - 18] { count++ }
                    if padded[baseIndex + 1 - 18] { count++ }
                    
                    // Middle row neighbors (left and right only)
                    if padded[baseIndex - 1] { count++ }
                    if padded[baseIndex + 1] { count++ }
                    
                    // Bottom row neighbors
                    if padded[baseIndex - 1 + 18] { count++ }
                    if padded[baseIndex + 18] { count++ }
                    if padded[baseIndex + 1 + 18] { count++ }
                    
                    // Apply Conway's Game of Life rules
                    current := out_q[i*16 + j]
                    switch count {
                    case 0, 1:
                        next_q[i*16 + j] = false // Underpopulation
                    case 2:
                        next_q[i*16 + j] = current // Stasis
                    case 3:
                        next_q[i*16 + j] = true // Reproduction
                    default: // 4-8
                        next_q[i*16 + j] = false // Overpopulation
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
