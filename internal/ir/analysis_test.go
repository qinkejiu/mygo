package ir

import (
	"strings"
	"testing"
)

const repeatedGoroutineChannelsProgram = `
package main

func writer(out chan<- int32) {
	var v int32
	v = 1
	out <- v
}

func reader(in <-chan int32) {
	var v int32
	v = <-in
	_ = v
}

func main() {
	ch0 := make(chan int32, 1)
	ch1 := make(chan int32, 1)
	go writer(ch0)
	go reader(ch0)
	go writer(ch1)
	go reader(ch1)
}
`

const sameGoroutineSPSCProgram = `
package main

func main() {
	ch := make(chan int32, 1)
	var v int32
	v = 7
	ch <- v
	v = <-ch
	_ = v
}
`

const multiProducerProgram = `
package main

func writer0(out chan<- int32) {
	var v int32
	v = 1
	out <- v
}

func writer1(out chan<- int32) {
	var v int32
	v = 2
	out <- v
}

func reader(in <-chan int32) {
	var v int32
	v = <-in
	_ = v
}

func main() {
	ch := make(chan int32, 1)
	go writer0(ch)
	go writer1(ch)
	go reader(ch)
}
`

func TestChannelAddEndpointDeduplicates(t *testing.T) {
	ch := &Channel{Name: "ch"}
	proc := &Process{Name: "p"}

	ch.AddEndpoint(proc, ChannelSend)
	ch.AddEndpoint(proc, ChannelSend)
	ch.AddEndpoint(proc, ChannelReceive)
	ch.AddEndpoint(proc, ChannelReceive)

	if len(ch.Producers) != 1 {
		t.Fatalf("expected 1 producer endpoint, got %d", len(ch.Producers))
	}
	if len(ch.Consumers) != 1 {
		t.Fatalf("expected 1 consumer endpoint, got %d", len(ch.Consumers))
	}
}

func TestCallGraphChannelBindingsDependenciesAndDepth(t *testing.T) {
	design := buildDesignFromSource(t, repeatedGoroutineChannelsProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design with top-level module")
	}

	checked := 0
	for _, ch := range design.TopLevel.Channels {
		if ch == nil || ch.DeclaredDepth != 1 {
			continue
		}
		checked++
		if !containsProcess(ch.Producers, "writer") {
			t.Fatalf("channel %s missing writer producer endpoint", ch.Name)
		}
		if !containsProcess(ch.Consumers, "reader") {
			t.Fatalf("channel %s missing reader consumer endpoint", ch.Name)
		}
		if len(ch.Dependencies) != 1 {
			t.Fatalf("channel %s expected exactly 1 dependency, got %d", ch.Name, len(ch.Dependencies))
		}
		dep := ch.Dependencies[0]
		if dep.Producer == nil || dep.Producer.Name != "writer" {
			t.Fatalf("channel %s unexpected dependency producer %v", ch.Name, dep.Producer)
		}
		if dep.Consumer == nil || dep.Consumer.Name != "reader" {
			t.Fatalf("channel %s unexpected dependency consumer %v", ch.Name, dep.Consumer)
		}
		if ch.Depth != 2 {
			t.Fatalf("channel %s expected inferred depth 2, got %d", ch.Name, ch.Depth)
		}
		if ch.InferredDepth != 2 {
			t.Fatalf("channel %s expected inferred depth metadata 2, got %d", ch.Name, ch.InferredDepth)
		}
		if !strings.Contains(ch.DepthReason, "SPSC cross-goroutine") {
			t.Fatalf("channel %s expected SPSC cross-goroutine reason, got %q", ch.Name, ch.DepthReason)
		}
	}

	if checked != 2 {
		t.Fatalf("expected 2 declared channels to validate, got %d", checked)
	}
}

func TestInferChannelDepthKeepsSameGoroutineSPSC(t *testing.T) {
	design := buildDesignFromSource(t, sameGoroutineSPSCProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design with top-level module")
	}

	for _, ch := range design.TopLevel.Channels {
		if ch == nil {
			continue
		}
		if ch.DeclaredDepth != 1 {
			t.Fatalf("channel %s expected declared depth 1, got %d", ch.Name, ch.DeclaredDepth)
		}
		if ch.InferredDepth != 1 {
			t.Fatalf("channel %s expected inferred depth 1, got %d", ch.Name, ch.InferredDepth)
		}
		if ch.Depth != 1 {
			t.Fatalf("channel %s expected final depth 1, got %d", ch.Name, ch.Depth)
		}
		if !strings.Contains(ch.DepthReason, "same goroutine") {
			t.Fatalf("channel %s expected same-goroutine reason, got %q", ch.Name, ch.DepthReason)
		}
		return
	}
	t.Fatalf("expected at least one channel")
}

func TestInferChannelDepthBumpsMultiProducer(t *testing.T) {
	design := buildDesignFromSource(t, multiProducerProgram)
	if design == nil || design.TopLevel == nil {
		t.Fatalf("expected design with top-level module")
	}

	for _, ch := range design.TopLevel.Channels {
		if ch == nil {
			continue
		}
		if ch.DeclaredDepth != 1 {
			t.Fatalf("channel %s expected declared depth 1, got %d", ch.Name, ch.DeclaredDepth)
		}
		if ch.InferredDepth != 2 {
			t.Fatalf("channel %s expected inferred depth 2, got %d", ch.Name, ch.InferredDepth)
		}
		if ch.Depth != 2 {
			t.Fatalf("channel %s expected final depth 2, got %d", ch.Name, ch.Depth)
		}
		if !strings.Contains(ch.DepthReason, "Multi-producer") {
			t.Fatalf("channel %s expected multi-producer reason, got %q", ch.Name, ch.DepthReason)
		}
		return
	}
	t.Fatalf("expected at least one channel")
}

func containsProcess(endpoints []*ChannelEndpoint, name string) bool {
	for _, endpoint := range endpoints {
		if endpoint != nil && endpoint.Process != nil && endpoint.Process.Name == name {
			return true
		}
	}
	return false
}
