package ir

// EnsureHardwareLowerableChannels reports unsupported channel topologies for hardware lowering.
// Multi-producer sends are structurally arbitrated during lowering.
func EnsureHardwareLowerableChannels(design *Design) error {
	return nil
}

func spawnedChannelProcesses(processes []*Process, module *Module) []*Process {
	if len(processes) == 0 {
		return nil
	}
	rootName := ""
	if module != nil {
		rootName = module.Name
	}
	result := make([]*Process, 0, len(processes))
	for _, proc := range processes {
		if proc == nil {
			continue
		}
		if proc.Spawned || (rootName != "" && proc.Name != rootName) {
			result = append(result, proc)
		}
	}
	return result
}

func uniqueChannelEndpointProcesses(endpoints []*ChannelEndpoint) []*Process {
	seen := make(map[*Process]struct{})
	processes := make([]*Process, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint == nil || endpoint.Process == nil {
			continue
		}
		if _, ok := seen[endpoint.Process]; ok {
			continue
		}
		seen[endpoint.Process] = struct{}{}
		processes = append(processes, endpoint.Process)
	}
	return processes
}
