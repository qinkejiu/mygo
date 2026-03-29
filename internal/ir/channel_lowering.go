package ir

import (
	"fmt"
	"sort"
	"strings"
)

// LoweredWire describes a lowered structural wire used to connect channel FIFOs.
type LoweredWire struct {
	Name  string
	Type  *SignalType
	InOut bool
}

// Connect records a structural connection between two lowered signals.
type Connect struct {
	Dst  string
	Src  string
	Type *SignalType
}

// PortBinding maps a lowered signal onto a FIFO instance port.
type PortBinding struct {
	Port  string
	Wire  string
	Type  *SignalType
	InOut bool
}

// FIFOInstance describes a structural FIFO instance and its bound ports.
type FIFOInstance struct {
	Name       string
	ModuleName string
	Ports      []PortBinding
}

// FIFODecl describes a concrete FIFO module required by the lowered design.
type FIFODecl struct {
	ModuleName            string
	ReusableModuleName    string
	DataType              *SignalType
	DataWidth             int
	Depth                 int
	AsyncReset            bool
	AlmostFullLevel       int
	AlmostEmptyLevel      int
	AddrWidth             int
	CountWidth            int
	LastPtrValue          int
	DepthCountValue       int
	AlmostFullCountValue  int
	AlmostEmptyCountValue int
	UseRegisteredRead     bool
	AlmostEmptyUsesEmpty  bool
}

// FIFOChannelWires groups the inout wires used to connect processes to a FIFO.
type FIFOChannelWires struct {
	WriteData   LoweredWire
	WriteValid  LoweredWire
	WriteReady  LoweredWire
	ReadData    LoweredWire
	ReadValid   LoweredWire
	ReadReady   LoweredWire
	Full        LoweredWire
	AlmostFull  LoweredWire
	Empty       LoweredWire
	AlmostEmpty LoweredWire
}

// FIFOChannelHelpers lists internal helper signals needed to drive FIFO control.
type FIFOChannelHelpers struct {
	OneConst   string
	ResetN     string
	FullValue  string
	NotFull    string
	EmptyValue string
	NotEmpty   string
}

// FIFOProducerWires groups the per-producer access wires used before arbitration.
type FIFOProducerWires struct {
	WriteData  LoweredWire
	WriteValid LoweredWire
	WriteReady LoweredWire
}

// LoweredChannelProducer records the dedicated write-side access for one producer.
type LoweredChannelProducer struct {
	Process *Process
	Wires   FIFOProducerWires
}

// LoweredChannelFIFO is the structural lowering product for one high-level channel.
type LoweredChannelFIFO struct {
	Channel   *Channel
	Decl      *FIFODecl
	Wires     FIFOChannelWires
	Helpers   FIFOChannelHelpers
	Instance  FIFOInstance
	Connects  []Connect
	Producers []*LoweredChannelProducer
}

// LoweredChannelModule contains the lowered FIFO structure for one module.
type LoweredChannelModule struct {
	Module *Module
	FIFOs  []*LoweredChannelFIFO
	byChan map[*Channel]*LoweredChannelFIFO
}

// LoweredChannelDesign is the dedicated lowering result consumed by emitters.
type LoweredChannelDesign struct {
	Modules   map[*Module]*LoweredChannelModule
	FIFODecls []*FIFODecl
}

// ModuleFor returns the lowered FIFO view for the given module.
func (l *LoweredChannelDesign) ModuleFor(module *Module) *LoweredChannelModule {
	if l == nil || module == nil {
		return nil
	}
	return l.Modules[module]
}

// FIFOFor returns the lowered FIFO metadata for the given channel.
func (m *LoweredChannelModule) FIFOFor(ch *Channel) *LoweredChannelFIFO {
	if m == nil || ch == nil {
		return nil
	}
	if m.byChan == nil {
		return nil
	}
	return m.byChan[ch]
}

// ProducerFor returns the per-producer write access metadata for the given process.
func (f *LoweredChannelFIFO) ProducerFor(proc *Process) *LoweredChannelProducer {
	if f == nil || proc == nil {
		return nil
	}
	for _, producer := range f.Producers {
		if producer != nil && producer.Process == proc {
			return producer
		}
	}
	return nil
}

// LowerChannelsToFIFO lowers high-level channel semantics into structural FIFO declarations,
// instances, wires, and explicit bindings for backend emitters.
func LowerChannelsToFIFO(design *Design) *LoweredChannelDesign {
	lowered := &LoweredChannelDesign{
		Modules: make(map[*Module]*LoweredChannelModule),
	}
	if design == nil {
		return lowered
	}

	declsByName := make(map[string]*FIFODecl)
	for _, module := range design.Modules {
		if module == nil {
			continue
		}
		loweredModule := &LoweredChannelModule{Module: module}
		names := make([]string, 0, len(module.Channels))
		for name := range module.Channels {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ch := module.Channels[name]
			if ch == nil {
				continue
			}
			fifo := lowerChannelFIFO(ch, declsByName)
			loweredModule.FIFOs = append(loweredModule.FIFOs, fifo)
			if loweredModule.byChan == nil {
				loweredModule.byChan = make(map[*Channel]*LoweredChannelFIFO)
			}
			loweredModule.byChan[ch] = fifo
		}
		lowered.Modules[module] = loweredModule
	}

	lowered.FIFODecls = make([]*FIFODecl, 0, len(declsByName))
	for _, decl := range declsByName {
		lowered.FIFODecls = append(lowered.FIFODecls, decl)
	}
	sort.Slice(lowered.FIFODecls, func(i, j int) bool {
		return lowered.FIFODecls[i].ModuleName < lowered.FIFODecls[j].ModuleName
	})
	return lowered
}

func lowerChannelFIFO(ch *Channel, declsByName map[string]*FIFODecl) *LoweredChannelFIFO {
	dataType := normalizedSignalType(ch.Type)
	boolType := &SignalType{Width: 1}
	depth := normalizedFIFODepth(ch.Depth)
	moduleName := loweredFIFOModuleName(dataType, depth)
	decl := declsByName[moduleName]
	if decl == nil {
		addrWidth := fifoAddrWidth(depth)
		countWidth := fifoCountWidth(depth)
		almostFullLevel := defaultAlmostFullLevel(depth)
		almostEmptyLevel := defaultAlmostEmptyLevel(depth)
		decl = &FIFODecl{
			ModuleName:            moduleName,
			ReusableModuleName:    reusableFIFOModuleName(),
			DataType:              dataType.Clone(),
			DataWidth:             signalWidth(dataType),
			Depth:                 depth,
			AsyncReset:            false,
			AlmostFullLevel:       almostFullLevel,
			AlmostEmptyLevel:      almostEmptyLevel,
			AddrWidth:             addrWidth,
			CountWidth:            countWidth,
			LastPtrValue:          depth - 1,
			DepthCountValue:       depth,
			AlmostFullCountValue:  almostFullLevel,
			AlmostEmptyCountValue: almostEmptyLevel,
			UseRegisteredRead:     depth > 64,
			AlmostEmptyUsesEmpty:  depth <= 1,
		}
		declsByName[moduleName] = decl
	}

	name := sanitizeIdentifier(ch.Name)
	wires := FIFOChannelWires{
		WriteData:   LoweredWire{Name: "chan_" + name + "_wdata", Type: dataType.Clone(), InOut: true},
		WriteValid:  LoweredWire{Name: "chan_" + name + "_wvalid", Type: boolType.Clone(), InOut: true},
		WriteReady:  LoweredWire{Name: "chan_" + name + "_wready", Type: boolType.Clone(), InOut: true},
		ReadData:    LoweredWire{Name: "chan_" + name + "_rdata", Type: dataType.Clone(), InOut: true},
		ReadValid:   LoweredWire{Name: "chan_" + name + "_rvalid", Type: boolType.Clone(), InOut: true},
		ReadReady:   LoweredWire{Name: "chan_" + name + "_rready", Type: boolType.Clone(), InOut: true},
		Full:        LoweredWire{Name: "chan_" + name + "_full", Type: boolType.Clone(), InOut: true},
		AlmostFull:  LoweredWire{Name: "chan_" + name + "_almost_full", Type: boolType.Clone(), InOut: true},
		Empty:       LoweredWire{Name: "chan_" + name + "_empty", Type: boolType.Clone(), InOut: true},
		AlmostEmpty: LoweredWire{Name: "chan_" + name + "_almost_empty", Type: boolType.Clone(), InOut: true},
	}
	helpers := FIFOChannelHelpers{
		OneConst:   "chan_" + name + "_one",
		ResetN:     "chan_" + name + "_rst_n",
		FullValue:  "chan_" + name + "_full_val",
		NotFull:    "chan_" + name + "_not_full",
		EmptyValue: "chan_" + name + "_empty_val",
		NotEmpty:   "chan_" + name + "_not_empty",
	}
	instance := FIFOInstance{
		Name:       name + "_fifo",
		ModuleName: decl.ModuleName,
		Ports: []PortBinding{
			{Port: "clk", Wire: "clk", Type: boolType.Clone()},
			{Port: "rst_n", Wire: helpers.ResetN, Type: boolType.Clone()},
			{Port: "wr_en", Wire: wires.WriteValid.Name, Type: boolType.Clone(), InOut: true},
			{Port: "wr_data", Wire: wires.WriteData.Name, Type: dataType.Clone(), InOut: true},
			{Port: "full", Wire: wires.Full.Name, Type: boolType.Clone(), InOut: true},
			{Port: "almost_full", Wire: wires.AlmostFull.Name, Type: boolType.Clone(), InOut: true},
			{Port: "rd_en", Wire: wires.ReadReady.Name, Type: boolType.Clone(), InOut: true},
			{Port: "rd_data", Wire: wires.ReadData.Name, Type: dataType.Clone(), InOut: true},
			{Port: "empty", Wire: wires.Empty.Name, Type: boolType.Clone(), InOut: true},
			{Port: "almost_empty", Wire: wires.AlmostEmpty.Name, Type: boolType.Clone(), InOut: true},
		},
	}
	connects := []Connect{
		{Dst: wires.WriteReady.Name, Src: helpers.NotFull, Type: boolType.Clone()},
		{Dst: wires.ReadValid.Name, Src: helpers.NotEmpty, Type: boolType.Clone()},
	}
	producers := lowerChannelProducers(ch, dataType, boolType)
	return &LoweredChannelFIFO{
		Channel:   ch,
		Decl:      decl,
		Wires:     wires,
		Helpers:   helpers,
		Instance:  instance,
		Connects:  connects,
		Producers: producers,
	}
}

func lowerChannelProducers(ch *Channel, dataType, boolType *SignalType) []*LoweredChannelProducer {
	if ch == nil {
		return nil
	}
	processes := uniqueChannelEndpointProcesses(ch.Producers)
	sort.SliceStable(processes, func(i, j int) bool {
		return loweredProcessLess(processes[i], processes[j])
	})
	name := sanitizeIdentifier(ch.Name)
	producers := make([]*LoweredChannelProducer, 0, len(processes))
	for idx, proc := range processes {
		if proc == nil {
			continue
		}
		producerPrefix := fmt.Sprintf("chan_%s_prod%d_%s", name, idx, sanitizeIdentifier(proc.Name))
		producers = append(producers, &LoweredChannelProducer{
			Process: proc,
			Wires: FIFOProducerWires{
				WriteData:  LoweredWire{Name: producerPrefix + "_wdata", Type: dataType.Clone(), InOut: true},
				WriteValid: LoweredWire{Name: producerPrefix + "_wvalid", Type: boolType.Clone(), InOut: true},
				WriteReady: LoweredWire{Name: producerPrefix + "_wready", Type: boolType.Clone(), InOut: true},
			},
		})
	}
	return producers
}

func loweredProcessLess(a, b *Process) bool {
	if a == nil || b == nil {
		return a != nil
	}
	aStage := a.Stage
	if aStage < 0 {
		aStage = 0
	}
	bStage := b.Stage
	if bStage < 0 {
		bStage = 0
	}
	if aStage != bStage {
		return aStage < bStage
	}
	aName := sanitizeIdentifier(a.Name)
	bName := sanitizeIdentifier(b.Name)
	if aName != bName {
		return aName < bName
	}
	return false
}

func normalizedSignalType(t *SignalType) *SignalType {
	if t == nil {
		return &SignalType{Width: 1}
	}
	clone := t.Clone()
	if clone.Width <= 0 {
		clone.Width = 1
	}
	return clone
}

func signalWidth(t *SignalType) int {
	if t == nil || t.Width <= 0 {
		return 1
	}
	return t.Width
}

func normalizedFIFODepth(depth int) int {
	if depth <= 0 {
		return 1
	}
	return depth
}

func loweredFIFOModuleName(t *SignalType, depth int) string {
	return fmt.Sprintf("mygo_fifo_%s_d%d", sanitizeIdentifier(loweredTypeString(t)), normalizedFIFODepth(depth))
}

func loweredTypeString(t *SignalType) string {
	return fmt.Sprintf("i%d", signalWidth(t))
}

func defaultAlmostFullLevel(depth int) int {
	if depth <= 1 {
		return 1
	}
	return depth - 1
}

func defaultAlmostEmptyLevel(depth int) int {
	if depth <= 1 {
		return 0
	}
	return 1
}

func fifoAddrWidth(depth int) int {
	if depth <= 1 {
		return 1
	}
	width := 0
	for value := depth - 1; value > 0; value >>= 1 {
		width++
	}
	if width == 0 {
		return 1
	}
	return width
}

func fifoCountWidth(depth int) int {
	if depth <= 1 {
		return 1
	}
	width := 0
	for value := depth; value > 0; value >>= 1 {
		width++
	}
	if width == 0 {
		return 1
	}
	return width
}

func reusableFIFOModuleName() string {
	return "mygo_fifo"
}

func sanitizeIdentifier(name string) string {
	if name == "" {
		return "unnamed"
	}
	var b strings.Builder
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (r >= '0' && r <= '9' && i > 0) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
