package main

func producer(out chan<- int32) {
	var v int32
	v = 7
	out <- v
}

func router(in <-chan int32, left chan<- int32, right chan<- int32) {
	var v int32
	v = <-in
	left <- v
	right <- v
}

func consumer(in <-chan int32) {
	var v int32
	v = <-in
	_ = v
}

func main() {
	inbound := make(chan int32, 2)
	left := make(chan int32, 1)
	right := make(chan int32, 1)
	go producer(inbound)
	go router(inbound, left, right)
	go consumer(left)
	go consumer(right)
}
