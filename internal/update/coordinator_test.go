package update

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeUpdater struct {
	mu                       sync.Mutex
	status                   Status
	result                   Result
	statusCalls, updateCalls int
}

func (fake *fakeUpdater) Status(context.Context) Status {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.statusCalls++
	return fake.status
}
func (fake *fakeUpdater) Update(context.Context) Result {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.updateCalls++
	return fake.result
}

func TestCoordinatorWaitsForActiveSessionsAndRestartsOnce(t *testing.T) {
	updater := &fakeUpdater{result: Result{State: "updated", Status: Status{CurrentSHA: "old", TargetSHA: "new"}, Message: "Updated to new"}}
	var active atomic.Int32
	active.Store(1)
	restarted := make(chan struct{}, 1)
	coordinator := NewCoordinator(updater, func() error { restarted <- struct{}{}; return nil }, func() int { return int(active.Load()) }, nil)
	coordinator.restartDelay = 0
	coordinator.waitInterval = time.Millisecond
	snapshot := coordinator.Start()
	if snapshot.State != "waiting" || snapshot.ActiveSessionCount == nil || *snapshot.ActiveSessionCount != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	active.Store(0)
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart timed out")
	}
	updater.mu.Lock()
	calls := updater.updateCalls
	updater.mu.Unlock()
	if calls != 1 {
		t.Fatalf("update calls = %d", calls)
	}
}

func TestCoordinatorWaitsForRestartAdmissionAfterTheIdleCheck(t *testing.T) {
	updater := &fakeUpdater{result: Result{State: "updated"}}
	admitted := atomic.Bool{}
	restarted := make(chan struct{}, 1)
	coordinator := NewCoordinator(updater, func() error { restarted <- struct{}{}; return nil }, nil, admitted.Load)
	coordinator.restartDelay = 0
	coordinator.waitInterval = time.Millisecond
	coordinator.Start()
	select {
	case <-restarted:
		t.Fatal("restarted before admission closed")
	case <-time.After(25 * time.Millisecond):
	}
	admitted.Store(true)
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart did not continue after admission closed")
	}
}

func TestCoordinatorDoesNotWedgeWhenInitialActivityCountPanics(t *testing.T) {
	calls := 0
	coordinator := NewCoordinator(&fakeUpdater{}, func() error { return nil }, func() int {
		calls++
		if calls == 1 {
			panic("activity panic")
		}
		return 0
	}, nil)
	func() {
		defer func() { _ = recover() }()
		coordinator.Start()
	}()
	if snapshot := coordinator.Start(); snapshot.State != "updating" {
		t.Fatalf("retry snapshot = %+v", snapshot)
	}
}

func TestCoordinatorSerializesConcurrentStarts(t *testing.T) {
	release := make(chan struct{})
	updater := &fakeUpdater{}
	coordinator := NewCoordinator(updater, func() error { return nil }, nil, nil)
	coordinator.updater = &blockingUpdater{fakeUpdater: updater, release: release}
	var wait sync.WaitGroup
	for range 20 {
		wait.Add(1)
		go func() { defer wait.Done(); coordinator.Start() }()
	}
	wait.Wait()
	time.Sleep(10 * time.Millisecond)
	updater.mu.Lock()
	calls := updater.updateCalls
	updater.mu.Unlock()
	if calls != 1 {
		t.Fatalf("update calls = %d", calls)
	}
	close(release)
}

type blockingUpdater struct {
	*fakeUpdater
	release chan struct{}
}

func (updater *blockingUpdater) Update(ctx context.Context) Result {
	updater.fakeUpdater.mu.Lock()
	updater.updateCalls++
	updater.fakeUpdater.mu.Unlock()
	select {
	case <-updater.release:
		return Result{State: "blocked"}
	case <-ctx.Done():
		return Result{State: "error", Message: ctx.Err().Error()}
	}
}

type panicUpdater struct {
	panicStatus bool
	panicUpdate bool
}

func (updater *panicUpdater) Status(context.Context) Status {
	if updater.panicStatus {
		updater.panicStatus = false
		panic("status panic")
	}
	return Status{State: "up_to_date"}
}
func (updater *panicUpdater) Update(context.Context) Result {
	if updater.panicUpdate {
		updater.panicUpdate = false
		panic("update panic")
	}
	return Result{State: "blocked"}
}

func TestCoordinatorReleasesOperationLockAfterUpdaterPanics(t *testing.T) {
	updater := &panicUpdater{panicStatus: true, panicUpdate: true}
	coordinator := NewCoordinator(updater, func() error { return nil }, nil, nil)
	if snapshot := coordinator.Status(); snapshot.State != "error" || snapshot.Message == nil || !strings.Contains(*snapshot.Message, "status panic") {
		t.Fatalf("status panic snapshot = %+v", snapshot)
	}
	if snapshot := coordinator.Status(); snapshot.State != "up_to_date" {
		t.Fatalf("status retry snapshot = %+v", snapshot)
	}
	coordinator.Start()
	deadline := time.Now().Add(time.Second)
	for coordinator.CachedStatus().State != "error" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	coordinator.Start()
	deadline = time.Now().Add(time.Second)
	for coordinator.CachedStatus().State == "updating" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if snapshot := coordinator.CachedStatus(); snapshot.State != "blocked" {
		t.Fatalf("update retry snapshot = %+v", snapshot)
	}
}

func TestCoordinatorLeavesUpdatingStateWithUsefulErrorWhenUpdateTimesOut(t *testing.T) {
	updater := &fakeUpdater{}
	coordinator := NewCoordinator(updater, func() error { return nil }, nil, nil)
	coordinator.updater = &blockingUpdater{fakeUpdater: updater, release: make(chan struct{})}
	coordinator.updateTimeout = 20 * time.Millisecond
	coordinator.Start()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := coordinator.CachedStatus()
		if snapshot.State == "error" {
			if snapshot.Message == nil || !strings.Contains(*snapshot.Message, "deadline exceeded") {
				t.Fatalf("snapshot = %+v", snapshot)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("coordinator remained in %+v", coordinator.CachedStatus())
}
