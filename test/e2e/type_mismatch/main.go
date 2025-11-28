package main

import "fmt"

// This workload mixes several integer widths to ensure the width inference
// pass can reconcile them without requiring redundant user annotations.
func main() {
	var small uint8 = 5
	var mid int16 = -12
	var wide uint32 = 1024

	acc := int16(small) + mid
	wide = uint32(acc) + wide

	fmt.Printf("acc=%d wide=%d\n", acc, wide)
}
