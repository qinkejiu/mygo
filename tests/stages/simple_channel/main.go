package main

import "fmt"

func worker(in <-chan int32, out chan<- int32) {
	v := <-in
	result := v + 1
	fmt.Printf("worker received=%d produced=%d\n", v, result)
	out <- result
}

func main() {
	in := make(chan int32, 4)
	out := make(chan int32, 4)

	go worker(in, out)

	in <- 5
	value := <-out
	fmt.Printf("main observed=%d\n", value)
}
