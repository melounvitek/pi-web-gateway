package update

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type updaterAPI interface {
	Status(context.Context) Status
	Update(context.Context) Result
}

type Snapshot struct {
	State              string
	Reason             *string
	Message            *string
	CurrentSHA         *string
	TargetSHA          *string
	BehindCount        *int
	Summary            *string
	ActiveSessionCount *int
}

type Coordinator struct {
	updater            updaterAPI
	restarter          func() error
	activeSessionCount func() int
	admitRestart       func() bool
	restartDelay       time.Duration
	waitInterval       time.Duration
	statusTimeout      time.Duration
	updateTimeout      time.Duration
	sleep              func(time.Duration)
	mu                 sync.Mutex
	operationMu        sync.Mutex
	snapshot           Snapshot
	statusChecking     bool
	running            bool
	finished           bool
	restartPending     bool
}

func NewCoordinator(updater updaterAPI, restarter func() error, active func() int, admitRestart func() bool) *Coordinator {
	if active == nil {
		active = func() int { return 0 }
	}
	if admitRestart == nil {
		admitRestart = func() bool { return true }
	}
	return &Coordinator{updater: updater, restarter: restarter, activeSessionCount: active, admitRestart: admitRestart, restartDelay: time.Second, waitInterval: time.Second, statusTimeout: 3 * time.Minute, updateTimeout: 25 * time.Minute, sleep: time.Sleep, snapshot: Snapshot{State: "unknown"}}
}

func (coordinator *Coordinator) CachedStatus() Snapshot {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.snapshot
}

func (coordinator *Coordinator) Status() Snapshot {
	coordinator.mu.Lock()
	if coordinator.running || coordinator.finished || coordinator.statusChecking {
		snapshot := coordinator.snapshot
		coordinator.mu.Unlock()
		return snapshot
	}
	coordinator.statusChecking = true
	coordinator.mu.Unlock()
	status := coordinator.checkStatus()
	snapshot := snapshotFromStatus(status)
	coordinator.mu.Lock()
	coordinator.statusChecking = false
	if !coordinator.running && !coordinator.finished {
		coordinator.snapshot = snapshot
	}
	snapshot = coordinator.snapshot
	coordinator.mu.Unlock()
	return snapshot
}

func (coordinator *Coordinator) Start() Snapshot {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.running {
		return coordinator.snapshot
	}
	active := coordinator.activeSessionCount()
	coordinator.running = true
	coordinator.finished = false
	restart := coordinator.restartPending
	if active > 0 {
		coordinator.snapshot = coordinator.waitingSnapshot(active)
	} else if restart {
		coordinator.snapshot = coordinator.restartingSnapshot()
	} else {
		coordinator.snapshot = coordinator.progressSnapshot()
	}
	go coordinator.background(restart)
	return coordinator.snapshot
}

func (coordinator *Coordinator) background(restart bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			coordinator.finish(coordinator.failure(fmt.Errorf("%v", recovered), map[bool]string{true: "restart", false: "update"}[restart]))
		}
	}()
	if restart {
		coordinator.performRestart()
		return
	}
	coordinator.performUpdate()
}

func (coordinator *Coordinator) performUpdate() {
	coordinator.waitForIdle()
	coordinator.mu.Lock()
	coordinator.snapshot = coordinator.progressSnapshot()
	coordinator.mu.Unlock()
	result := coordinator.runUpdate()
	if result.State != "updated" {
		coordinator.finish(snapshotFromResult(result))
		return
	}
	coordinator.mu.Lock()
	coordinator.restartPending = true
	coordinator.snapshot = Snapshot{State: "restarting", Message: stringPointer(nonempty(result.Message, "Restarting gateway…")), CurrentSHA: stringPointerOrNil(result.Status.CurrentSHA), TargetSHA: stringPointerOrNil(result.Status.TargetSHA), BehindCount: intPointer(result.Status.BehindCount), Summary: stringPointerOrNil(result.Status.Summary)}
	coordinator.mu.Unlock()
	coordinator.performRestart()
}

func (coordinator *Coordinator) checkStatus() (status Status) {
	coordinator.operationMu.Lock()
	defer coordinator.operationMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), coordinator.statusTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			status = Status{State: "error", Reason: "check", Message: fmt.Sprint(recovered)}
		}
	}()
	status = coordinator.updater.Status(ctx)
	if ctx.Err() != nil {
		status = Status{State: "error", Reason: "timeout", Message: "Gateway update check timed out: " + ctx.Err().Error()}
	}
	return status
}

func (coordinator *Coordinator) runUpdate() Result {
	coordinator.operationMu.Lock()
	defer coordinator.operationMu.Unlock()
	timeout := coordinator.updateTimeout
	if timeout > 2*updateCleanupTimeout {
		timeout -= updateCleanupTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result := coordinator.updater.Update(ctx)
	if ctx.Err() != nil && result.State != "updated" {
		result = Result{State: "error", Status: result.Status, Message: "Gateway update timed out: " + ctx.Err().Error()}
	}
	return result
}

func (coordinator *Coordinator) performRestart() {
	coordinator.waitForIdle()
	coordinator.mu.Lock()
	coordinator.snapshot = coordinator.restartingSnapshot()
	coordinator.mu.Unlock()
	coordinator.sleep(coordinator.restartDelay)
	for {
		coordinator.waitForIdle()
		if coordinator.admitRestart() {
			break
		}
		coordinator.sleep(coordinator.waitInterval)
	}
	coordinator.mu.Lock()
	coordinator.snapshot = coordinator.restartingSnapshot()
	coordinator.mu.Unlock()
	if err := coordinator.restarter(); err != nil {
		coordinator.finish(coordinator.failure(err, "restart"))
	}
}

func (coordinator *Coordinator) waitForIdle() {
	for {
		active := coordinator.activeSessionCount()
		if active <= 0 {
			return
		}
		coordinator.mu.Lock()
		coordinator.snapshot = coordinator.waitingSnapshot(active)
		coordinator.mu.Unlock()
		coordinator.sleep(coordinator.waitInterval)
	}
}

func (coordinator *Coordinator) finish(snapshot Snapshot) {
	coordinator.mu.Lock()
	coordinator.snapshot = snapshot
	coordinator.running = false
	coordinator.finished = true
	coordinator.mu.Unlock()
}

func (coordinator *Coordinator) progressSnapshot() Snapshot {
	previous := coordinator.snapshot
	previous.State = "updating"
	previous.Reason = nil
	previous.Message = stringPointer("Updating gateway…")
	previous.ActiveSessionCount = nil
	return previous
}

func (coordinator *Coordinator) restartingSnapshot() Snapshot {
	previous := coordinator.snapshot
	alreadyRestarting := previous.State == "restarting"
	previous.State = "restarting"
	previous.Reason = nil
	if !alreadyRestarting {
		previous.Message = stringPointer("Restarting gateway…")
	}
	previous.ActiveSessionCount = nil
	return previous
}

func (coordinator *Coordinator) waitingSnapshot(active int) Snapshot {
	previous := coordinator.snapshot
	sessions := "sessions"
	if active == 1 {
		sessions = "session"
	}
	previous.State = "waiting"
	previous.Reason = nil
	previous.Message = stringPointer(fmt.Sprintf("Waiting for %d active Pi %s to finish…", active, sessions))
	previous.ActiveSessionCount = intPointer(active)
	return previous
}

func (coordinator *Coordinator) failure(err error, reason string) Snapshot {
	previous := coordinator.snapshot
	previous.State = "error"
	previous.Reason = stringPointer(reason)
	previous.Message = stringPointer(err.Error())
	previous.ActiveSessionCount = nil
	return previous
}

func snapshotFromStatus(status Status) Snapshot {
	snapshot := Snapshot{State: status.State, Reason: stringPointerOrNil(status.Reason), Message: stringPointerOrNil(status.Message), CurrentSHA: stringPointerOrNil(status.CurrentSHA), TargetSHA: stringPointerOrNil(status.TargetSHA), Summary: stringPointerOrNil(status.Summary)}
	if status.State == "available" || status.State == "up_to_date" || status.Reason == "ahead" || status.Reason == "diverged" {
		snapshot.BehindCount = intPointer(status.BehindCount)
	}
	return snapshot
}

func snapshotFromResult(result Result) Snapshot {
	snapshot := snapshotFromStatus(result.Status)
	snapshot.State = result.State
	snapshot.Message = stringPointerOrNil(result.Message)
	if snapshot.Reason == nil {
		snapshot.Reason = stringPointer(result.State)
	}
	return snapshot
}

func stringPointer(value string) *string { return &value }
func stringPointerOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func intPointer(value int) *int { return &value }
func nonempty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
