package resource

type Snapshot struct {
	MemoryBytes       int64
	WorkingSetBytes   int64
	InactiveFileBytes int64
	CPUUsageUsec      int64
	GatewayRSSBytes   int64
	PiRSSBytes        int64
	PiProcessCount    int
}
