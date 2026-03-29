package ir

import "testing"

func TestLowerChannelsToFIFOProducesStructuralFIFO(t *testing.T) {
	ch := &Channel{
		Name:  "data.ch",
		Type:  &SignalType{Width: 32},
		Depth: 4,
	}
	module := &Module{
		Name:     "top",
		Channels: map[string]*Channel{ch.Name: ch},
	}
	lowered := LowerChannelsToFIFO(&Design{Modules: []*Module{module}})
	loweredModule := lowered.ModuleFor(module)
	if loweredModule == nil {
		t.Fatalf("expected lowered module for %s", module.Name)
	}
	if len(loweredModule.FIFOs) != 1 {
		t.Fatalf("expected 1 lowered fifo, got %d", len(loweredModule.FIFOs))
	}

	fifo := loweredModule.FIFOs[0]
	if fifo.Decl == nil {
		t.Fatalf("expected fifo declaration")
	}
	if fifo.Decl.ModuleName != "mygo_fifo_i32_d4" {
		t.Fatalf("unexpected fifo module name: %s", fifo.Decl.ModuleName)
	}
	if fifo.Decl.ReusableModuleName != "mygo_fifo" {
		t.Fatalf("unexpected reusable fifo module name: %s", fifo.Decl.ReusableModuleName)
	}
	if fifo.Decl.AddrWidth != 2 || fifo.Decl.CountWidth != 3 {
		t.Fatalf("unexpected fifo sizing: addr=%d count=%d", fifo.Decl.AddrWidth, fifo.Decl.CountWidth)
	}
	if fifo.Decl.LastPtrValue != 3 || fifo.Decl.DepthCountValue != 4 {
		t.Fatalf("unexpected fifo wrap/count values: last=%d depth_count=%d", fifo.Decl.LastPtrValue, fifo.Decl.DepthCountValue)
	}
	if fifo.Decl.UseRegisteredRead {
		t.Fatalf("expected shallow fifo to avoid registered read path")
	}
	if fifo.Instance.Name != "data_ch_fifo" {
		t.Fatalf("unexpected fifo instance name: %s", fifo.Instance.Name)
	}
	if fifo.Wires.WriteData.Name != "chan_data_ch_wdata" {
		t.Fatalf("unexpected write data wire: %s", fifo.Wires.WriteData.Name)
	}
	if fifo.Helpers.ResetN != "chan_data_ch_rst_n" {
		t.Fatalf("unexpected reset helper: %s", fifo.Helpers.ResetN)
	}
	if len(fifo.Instance.Ports) != 10 {
		t.Fatalf("expected 10 port bindings, got %d", len(fifo.Instance.Ports))
	}
	if len(fifo.Connects) != 2 {
		t.Fatalf("expected 2 structural connects, got %d", len(fifo.Connects))
	}
	if fifo.Connects[0].Dst != fifo.Wires.WriteReady.Name || fifo.Connects[0].Src != fifo.Helpers.NotFull {
		t.Fatalf("unexpected write-ready connect: %+v", fifo.Connects[0])
	}
	if fifo.Connects[1].Dst != fifo.Wires.ReadValid.Name || fifo.Connects[1].Src != fifo.Helpers.NotEmpty {
		t.Fatalf("unexpected read-valid connect: %+v", fifo.Connects[1])
	}
}

func TestLowerChannelsToFIFODeduplicatesDeclarations(t *testing.T) {
	ch0 := &Channel{Name: "a", Type: &SignalType{Width: 8}, Depth: 2}
	ch1 := &Channel{Name: "b", Type: &SignalType{Width: 8}, Depth: 2}
	mod0 := &Module{Name: "m0", Channels: map[string]*Channel{"a": ch0}}
	mod1 := &Module{Name: "m1", Channels: map[string]*Channel{"b": ch1}}

	lowered := LowerChannelsToFIFO(&Design{Modules: []*Module{mod0, mod1}})
	if len(lowered.FIFODecls) != 1 {
		t.Fatalf("expected 1 shared fifo declaration, got %d", len(lowered.FIFODecls))
	}
	if lowered.FIFODecls[0].ModuleName != "mygo_fifo_i8_d2" {
		t.Fatalf("unexpected shared fifo declaration: %s", lowered.FIFODecls[0].ModuleName)
	}
	if lowered.FIFODecls[0].ReusableModuleName != "mygo_fifo" {
		t.Fatalf("expected shared reusable fifo module name, got %s", lowered.FIFODecls[0].ReusableModuleName)
	}
}

func TestLowerChannelsToFIFOAddsDeterministicMultiProducerAccessWires(t *testing.T) {
	consumer := &Process{Name: "main", Stage: 0}
	producerB := &Process{Name: "writer_b", Stage: 4, Spawned: true}
	producerA := &Process{Name: "writer_a", Stage: 2, Spawned: true}
	ch := &Channel{
		Name:  "done",
		Type:  &SignalType{Width: 1},
		Depth: 2,
		Producers: []*ChannelEndpoint{
			{Process: producerB, Direction: ChannelSend},
			{Process: producerA, Direction: ChannelSend},
		},
		Consumers: []*ChannelEndpoint{
			{Process: consumer, Direction: ChannelReceive},
		},
	}
	module := &Module{
		Name:     "top",
		Channels: map[string]*Channel{ch.Name: ch},
	}
	lowered := LowerChannelsToFIFO(&Design{Modules: []*Module{module}})
	fifo := lowered.ModuleFor(module).FIFOFor(ch)
	if fifo == nil {
		t.Fatalf("expected lowered fifo for channel %s", ch.Name)
	}
	if len(fifo.Producers) != 2 {
		t.Fatalf("expected 2 producer access records, got %d", len(fifo.Producers))
	}
	if fifo.Producers[0].Process != producerA {
		t.Fatalf("expected stage/name-sorted producer order, got first=%v", fifo.Producers[0].Process)
	}
	if fifo.Producers[1].Process != producerB {
		t.Fatalf("expected second producer to be writer_b, got %v", fifo.Producers[1].Process)
	}
	if got := fifo.Producers[0].Wires.WriteData.Name; got != "chan_done_prod0_writer_a_wdata" {
		t.Fatalf("unexpected first producer write-data wire: %s", got)
	}
	if got := fifo.Producers[1].Wires.WriteReady.Name; got != "chan_done_prod1_writer_b_wready" {
		t.Fatalf("unexpected second producer write-ready wire: %s", got)
	}
	if fifo.ProducerFor(producerA) == nil || fifo.ProducerFor(producerB) == nil {
		t.Fatalf("expected producer lookup to resolve dedicated access wires")
	}
}
