package main

import "fmt"

const loopTripCount = 4

// producer writes a short increasing sequence onto out.
func producer(out chan<- int) {
	for i := 0; i < loopTripCount; i++ {
		out <- i
		fmt.Printf("producer sent %d\n", i)
	}
}

// consumer drains exactly loopTripCount values and accumulates them.
func consumer(in <-chan int) int {
	total := 0
	for i := 0; i < loopTripCount; i++ {
		value := <-in
		total += value
		fmt.Printf("consumer received %d (running total %d)\n", value, total)
	}
	return total
}

func main() {
	stream := make(chan int, 1)

	go producer(stream)

	result := consumer(stream)

	// Keep the result live so the SSA builder materializes the loop state.
	// The value is intentionally unused otherwise to keep the program small.
	if result == 42 {
		stream <- result
	}

	fmt.Printf("phi loop final=%d\n", result)
}
