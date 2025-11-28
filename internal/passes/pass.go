package passes

import (
	"fmt"

	"mygo/internal/ir"
)

// Pass defines the interface for all optimization/analysis passes.
type Pass interface {
	Name() string
	Run(*ir.Design) error
}

// Manager holds an ordered list of passes.
type Manager struct {
	passes []Pass
}

// NewManager creates an empty pass manager.
func NewManager() *Manager {
	return &Manager{passes: make([]Pass, 0)}
}

// Add appends a pass to the execution list.
func (m *Manager) Add(p Pass) {
	if p == nil {
		return
	}
	m.passes = append(m.passes, p)
}

// Run executes each registered pass sequentially.
func (m *Manager) Run(design *ir.Design) error {
	if design == nil {
		return fmt.Errorf("nil design provided to pass manager")
	}
	for _, pass := range m.passes {
		if pass == nil {
			continue
		}
		if err := pass.Run(design); err != nil {
			return fmt.Errorf("pass %s failed: %w", pass.Name(), err)
		}
	}
	return nil
}
