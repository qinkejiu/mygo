package main

import "fmt"

const (
	numPackets = 4
	fieldSrc   = 0
	fieldDest  = 1
	fieldData  = 2
)

// Pack a three-field packet into a 32-bit word: [src|dest|data].
func makePacket(src, dest, data uint32) uint32 {
	return (src << 24) | (dest << 16) | (data & 0xFFFF)
}

func unpackPacket(pkt uint32) (src, dest, data uint32) {
	src = (pkt >> 24) & 0xFF
	dest = (pkt >> 16) & 0xFF
	data = pkt & 0xFFFF
	return
}

func producer(id uint32, baseDest uint32, out chan<- uint32) {
	for i := uint32(0); i < numPackets; i++ {
		dest := (baseDest + i) & 1
		pkt := makePacket(id, dest, id*10+i)
		out <- pkt
		fmt.Printf("producer %d sent dest=%d payload=%d\n", id, dest, id*10+i)
	}
}

func router(left, right <-chan uint32, outA, outB chan<- uint32) {
	for i := uint32(0); i < numPackets; i++ {
		routePacket(<-left, outA, outB)
		routePacket(<-right, outA, outB)
	}
}

func routePacket(pkt uint32, outA, outB chan<- uint32) {
	_, dest, _ := unpackPacket(pkt)
	if dest&1 == 0 {
		outA <- pkt
	} else {
		outB <- pkt
	}
}

func consumer(id uint32, in <-chan uint32, done chan<- bool) {
	for i := uint32(0); i < numPackets; i++ {
		pkt := <-in
		src, dest, data := unpackPacket(pkt)
		fmt.Printf("consumer %d got src=%d dest=%d payload=%d\n", id, src, dest, data)
	}
	done <- true
}

func main() {
	left := make(chan uint32, 1)
	right := make(chan uint32, 1)
	outA := make(chan uint32, 1)
	outB := make(chan uint32, 1)
	done := make(chan bool, 2)

	go consumer(0, outA, done)
	go consumer(1, outB, done)
	go router(left, right, outA, outB)
	go producer(0, 0, left)
	go producer(1, 1, right)

	for i := 0; i < 2; i++ {
		<-done
	}
	fmt.Println("router complete")
}
