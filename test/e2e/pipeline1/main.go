package main

func source(out chan<- int32) {
	var v int32
	v = 1
	out <- v
}

func middle(in <-chan int32, out chan<- int32) {
	var v int32
	v = <-in
	out <- v + 2
}

func sink(in <-chan int32) {
	var v int32
	v = <-in
	_ = v
}

func main() {
	ch0 := make(chan int32, 4)
	ch1 := make(chan int32, 4)
	go source(ch0)
	go middle(ch0, ch1)
	go sink(ch1)
}
