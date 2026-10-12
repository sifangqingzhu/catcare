package analysisjobs

import (
	"catcare/internal/analysis"
	"catcare/internal/model"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func snapshot() Snapshot {
	return Snapshot{Now: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), Timezone: "UTC", Records: []model.Record{{ID: 1}}}
}
func done(t *testing.T, m *Manager, table uint, id string) View {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, _, ok := m.Get(table, id)
		if !ok {
			t.Fatal("missing task")
		}
		if v.Status == "done" || v.Status == "failed" {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("task did not finish")
	return View{}
}
func TestSubmitReturnsWhileComputeWaitsAndDeduplicates(t *testing.T) {
	entered, release := make(chan Snapshot, 1), make(chan struct{})
	var calls atomic.Int32
	m := New(context.Background(), func(ctx context.Context, s Snapshot, p func(int, string)) (analysis.Report, error) {
		calls.Add(1)
		entered <- s
		p(40, "fit")
		select {
		case <-release:
		case <-ctx.Done():
			return analysis.Report{}, ctx.Err()
		}
		return analysis.Report{SharedCount: 7}, nil
	})
	defer m.Close()
	input := snapshot()
	v, result, err := m.Submit(1, input, false)
	if err != nil || result != nil {
		t.Fatal(err)
	}
	input.Records[0].ID = 99
	select {
	case copy := <-entered:
		if copy.Records[0].ID != 1 {
			t.Fatal("mutable input leaked")
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	for i := 0; i < 10; i++ {
		other, _, err := m.Submit(1, snapshot(), true)
		if err != nil || other.ID != v.ID {
			t.Fatal("active work was duplicated")
		}
	}
	status, _, _ := m.Get(1, v.ID)
	if status.Progress != 40 || status.Status != "running" {
		t.Fatal(status)
	}
	if _, _, ok := m.Get(2, v.ID); ok {
		t.Fatal("cross-table result leak")
	}
	close(release)
	if done(t, m, 1, v.ID).Progress != 100 {
		t.Fatal("missing completion")
	}
	cached, r, _ := m.Submit(1, snapshot(), false)
	if cached.ID != v.ID || r.SharedCount != 7 || calls.Load() != 1 {
		t.Fatal("cache was not reused")
	}
}
func TestChangedInputCancelsOldResult(t *testing.T) {
	entered, cancelled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	m := New(context.Background(), func(ctx context.Context, s Snapshot, p func(int, string)) (analysis.Report, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			return analysis.Report{SharedCount: 999}, nil
		}
		return analysis.Report{SharedCount: int(s.Records[0].ID)}, nil
	})
	defer m.Close()
	old, _, _ := m.Submit(1, snapshot(), false)
	<-entered
	next := snapshot()
	next.Records[0].ID = 2
	newer, _, _ := m.Submit(1, next, false)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("old fit not cancelled")
	}
	if _, _, ok := m.Get(1, old.ID); ok {
		t.Fatal("superseded job still readable")
	}
	done(t, m, 1, newer.ID)
	_, r, _ := m.Get(1, newer.ID)
	if r.SharedCount != 2 {
		t.Fatal("stale publication")
	}
	m.Invalidate(1)
	if _, _, ok := m.Get(1, newer.ID); ok {
		t.Fatal("mutation left stale result")
	}
}
func TestBoundedQueueAndShutdown(t *testing.T) {
	entered := make(chan struct{}, 1)
	m := New(context.Background(), func(ctx context.Context, s Snapshot, p func(int, string)) (analysis.Report, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return analysis.Report{}, ctx.Err()
	})
	m.Submit(1, snapshot(), false)
	<-entered
	for i := uint(2); i <= 33; i++ {
		v, _, _ := m.Submit(i, snapshot(), false)
		if v.Status != "queued" {
			t.Fatal(v)
		}
	}
	v, _, _ := m.Submit(34, snapshot(), false)
	if v.Status != "deferred" {
		t.Fatal("unbounded work", v)
	}
	m.Close()
	if _, _, err := m.Submit(1, snapshot(), false); err == nil {
		t.Fatal("closed manager accepted work")
	}
}
func TestFailureAndPanicDoNotStopWorker(t *testing.T) {
	var calls atomic.Int32
	m := New(context.Background(), func(ctx context.Context, s Snapshot, p func(int, string)) (analysis.Report, error) {
		switch calls.Add(1) {
		case 1:
			panic("secret internal failure")
		case 2:
			return analysis.Report{}, errors.New("secret")
		}
		return analysis.Report{}, nil
	})
	defer m.Close()
	for i := uint(1); i <= 3; i++ {
		v, _, _ := m.Submit(i, snapshot(), false)
		r := done(t, m, i, v.ID)
		want := "failed"
		if i == 3 {
			want = "done"
		}
		if r.Status != want || r.Error == "secret" {
			t.Fatal(r)
		}
	}
}
func TestConcurrentAccessIsSafe(t *testing.T) {
	m := New(context.Background(), func(ctx context.Context, s Snapshot, p func(int, string)) (analysis.Report, error) {
		p(10, "fit")
		p(2, "read")
		return analysis.Report{}, nil
	})
	defer m.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, _, _ := m.Submit(1, snapshot(), false)
			for k := 0; k < 20; k++ {
				m.Get(1, v.ID)
			}
		}()
	}
	wg.Wait()
	v, _, _ := m.Submit(1, snapshot(), false)
	if done(t, m, 1, v.ID).Progress != 100 {
		t.Fatal("progress regression")
	}
}
