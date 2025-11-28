package main

func stage1(out chan<- int32) {
	var v int32
	v = 1
	out <- v
}

func stage2(in <-chan int32, out chan<- int32) {
	var v int32
	v = <-in
	out <- v * 2
}

func stage3(in <-chan int32, out chan<- int32) {
	var v int32
	v = <-in
	out <- v - 1
}

func sink(in <-chan int32) {
	var v int32
	v = <-in
	_ = v
}

func main() {
	ch0 := make(chan int32, 2)
	ch1 := make(chan int32, 2)
	ch2 := make(chan int32, 2)
	go stage1(ch0)
	go stage2(ch0, ch1)
	go stage3(ch1, ch2)
	go sink(ch2)
}
