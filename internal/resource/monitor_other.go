//go:build !linux

package resource

type Monitor struct{}

func NewMonitor() *Monitor                    { return &Monitor{} }
func (*Monitor) Snapshot() (*Snapshot, error) { return nil, nil }
