package rpc

import (
	"context"
	"time"
)

type RPCClient interface {
	Close() error
	Busy() bool
	BusySince() *time.Time
	SettledAt() *time.Time
	AgentRunning() bool
	Compacting() bool
	EventSequence() int64
	EventReplayCursor() int64
	EventsAfter(int64) EventBatch
	LiveSnapshot() LiveSnapshot
	GetState(context.Context) (map[string]any, error)
	GetSessionStats(context.Context) (map[string]any, error)
	GetCommands(context.Context) (map[string]any, error)
	SessionPosition(context.Context, string) (SessionEntries, error)
	SessionEntriesAfter(context.Context, string) (SessionEntries, error)
}
