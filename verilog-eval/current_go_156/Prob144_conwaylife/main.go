package main

var out_q [256]bool

func TopModule(clk bool, load bool, data [256]bool) {
	if clk {
		if load {
			for i := 0; i < 256; i++ {
				out_q[i] = data[i]
			}
		} else {
			var next [256]bool
			for row := 0; row < 16; row++ {
				up := row - 1
				down := row + 1
				if row == 0 {
					up = 15
				}
				if row == 15 {
					down = 0
				}

				for col := 0; col < 16; col++ {
					left := col - 1
					right := col + 1
					if col == 0 {
						left = 15
					}
					if col == 15 {
						right = 0
					}

					count := 0
					if out_q[up*16+left] {
						count++
					}
					if out_q[up*16+col] {
						count++
					}
					if out_q[up*16+right] {
						count++
					}
					if out_q[row*16+left] {
						count++
					}
					if out_q[row*16+right] {
						count++
					}
					if out_q[down*16+left] {
						count++
					}
					if out_q[down*16+col] {
						count++
					}
					if out_q[down*16+right] {
						count++
					}

					idx := row*16 + col
					next[idx] = count == 3 || (out_q[idx] && count == 2)
				}
			}

			for i := 0; i < 256; i++ {
				out_q[i] = next[i]
			}
		}
	}
}

func main() {}
