package main

import "fmt"

const (
	numPackets = 4
	fieldSrc   = 0
	fieldDest  = 1
	fieldData  = 2
)

func producer0(out chan<- uint32) {
	for i := uint32(0); i < numPackets; i++ {
		dest := i & 1
		payload := i
		pkt := (uint32(0) << 24) | (dest << 16) | (payload & 0xFFFF)
		out <- pkt
		fmt.Printf("producer %d sent dest=%d payload=%d\n", uint32(0), dest, payload)
	}
}

func producer1(out chan<- uint32) {
	for i := uint32(0); i < numPackets; i++ {
		dest := (uint32(1) + i) & 1
		payload := uint32(10) + i
		pkt := (uint32(1) << 24) | (dest << 16) | (payload & 0xFFFF)
		out <- pkt
		fmt.Printf("producer %d sent dest=%d payload=%d\n", uint32(1), dest, payload)
	}
}

func router(left, right <-chan uint32, outA, outB chan<- uint32) {
	for i := uint32(0); i < numPackets; i++ {
		leftPkt := <-left
		leftDest := (leftPkt >> 16) & 0xFF
		if leftDest&1 == 0 {
			outA <- leftPkt
		} else {
			outB <- leftPkt
		}

		rightPkt := <-right
		rightDest := (rightPkt >> 16) & 0xFF
		if rightDest&1 == 0 {
			outA <- rightPkt
		} else {
			outB <- rightPkt
		}
	}
}

func consumer0(in <-chan uint32, done chan<- bool) {
	for i := uint32(0); i < numPackets; i++ {
		pkt := <-in
		src := (pkt >> 24) & 0xFF
		dest := (pkt >> 16) & 0xFF
		data := pkt & 0xFFFF
		fmt.Printf("consumer %d got src=%d dest=%d payload=%d\n", uint32(0), src, dest, data)
	}
	done <- true
}

func consumer1(in <-chan uint32, done chan<- bool) {
	for i := uint32(0); i < numPackets; i++ {
		pkt := <-in
		src := (pkt >> 24) & 0xFF
		dest := (pkt >> 16) & 0xFF
		data := pkt & 0xFFFF
		fmt.Printf("consumer %d got src=%d dest=%d payload=%d\n", uint32(1), src, dest, data)
	}
	done <- true
}

func main() {
	left := make(chan uint32, 1)
	right := make(chan uint32, 1)
	outA := make(chan uint32, 1)
	outB := make(chan uint32, 1)
	done := make(chan bool, 2)

	go consumer0(outA, done)
	go consumer1(outB, done)
	go router(left, right, outA, outB)
	go producer0(left)
	go producer1(right)

	for i := 0; i < 2; i++ {
		<-done
	}
	fmt.Printf("router complete packets=%d\n", numPackets)
}
