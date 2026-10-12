package analysisjobs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"catcare/internal/analysis"
	"catcare/internal/model"
)

type Snapshot struct {
	Records  []model.Record
	Weights  []model.Weight
	Boxes    []model.Box
	Litters  []model.Litter
	Timezone string
	Now      time.Time
}
type View struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	Phase     string    `json:"phase"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	HasResult bool      `json:"has_result"`
}
type Compute func(context.Context, Snapshot, func(int, string)) (analysis.Report, error)
type task struct {
	table    uint
	key      string
	view     View
	input    Snapshot
	report   *analysis.Report
	ctx      context.Context
	cancel   context.CancelFunc
	accessed time.Time
}
type Manager struct {
	mu      sync.Mutex
	latest  map[uint]*task
	queue   chan *task
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	compute Compute
}

func New(parent context.Context, compute Compute) *Manager {
	ctx, cancel := context.WithCancel(parent)
	m := &Manager{latest: map[uint]*task{}, queue: make(chan *task, 32), ctx: ctx, cancel: cancel, compute: compute}
	m.wg.Add(1)
	go m.worker()
	return m
}
func DefaultCompute(ctx context.Context, s Snapshot, p func(int, string)) (analysis.Report, error) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return analysis.Report{}, err
	}
	return analysis.AnalyzePoolsContext(ctx, s.Records, s.Weights, s.Boxes, s.Litters, s.Now, p, loc)
}
func (m *Manager) Close() { m.cancel(); m.wg.Wait() }
func (m *Manager) Submit(table uint, input Snapshot, force bool) (View, *analysis.Report, error) {
	// Immutable snapshots and a five-minute clock bucket invalidate time-derived
	// results. JSON copying happens before the short queue/cache lock.
	keyInput := input
	keyInput.Now = input.Now.Truncate(5 * time.Minute)
	raw, err := json.Marshal(keyInput)
	if err != nil {
		return View{}, nil, err
	}
	hash := sha256.Sum256(raw)
	key := hex.EncodeToString(hash[:])
	copied := Snapshot{}
	if err = json.Unmarshal(raw, &copied); err != nil {
		return View{}, nil, err
	}
	copied.Now = input.Now
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		return View{}, nil, m.ctx.Err()
	}
	if old := m.latest[table]; old != nil && old.key == key && (!force || old.view.Status == "running" || old.view.Status == "queued") {
		old.accessed = now
		return old.view, old.report, nil
	}
	// Evict completed/deferred entries; active work remains bounded by the queue.
	if len(m.latest) >= 128 {
		var oldest *task
		for _, t := range m.latest {
			if t.view.Status == "running" || t.view.Status == "queued" {
				continue
			}
			if oldest == nil || t.accessed.Before(oldest.accessed) {
				oldest = t
			}
		}
		if oldest != nil {
			oldest.cancel()
			delete(m.latest, oldest.table)
		}
	}
	token := make([]byte, 12)
	if _, err = rand.Read(token); err != nil {
		return View{}, nil, err
	}
	ctx, cancel := context.WithCancel(m.ctx)
	t := &task{table: table, key: key, input: copied, ctx: ctx, cancel: cancel, accessed: now, view: View{ID: hex.EncodeToString(token), Status: "queued", Phase: "等待后台分析", CreatedAt: now, UpdatedAt: now}}
	if old := m.latest[table]; old != nil {
		old.cancel()
	}
	m.latest[table] = t
	select {
	case m.queue <- t:
	default:
		t.view.Status = "deferred"
		t.view.Error = "分析队列繁忙，请稍后重试"
		t.input = Snapshot{}
		t.cancel()
	}
	return t.view, nil, nil
}
func (m *Manager) Get(table uint, id string) (View, *analysis.Report, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.latest[table]
	if t == nil || t.view.ID != id {
		return View{}, nil, false
	}
	t.accessed = time.Now()
	return t.view, t.report, true
}
func (m *Manager) Invalidate(table uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.latest[table]; t != nil {
		t.cancel()
		delete(m.latest, table)
	}
}
func (m *Manager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case t := <-m.queue:
			if t.ctx.Err() != nil {
				continue
			}
			m.run(t)
		}
	}
}
func (m *Manager) run(t *task) {
	m.mu.Lock()
	if m.latest[t.table] != t {
		m.mu.Unlock()
		return
	}
	t.view.Status = "running"
	t.view.Phase = "建立基准"
	t.view.UpdatedAt = time.Now()
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.ctx, 2*time.Minute)
	defer cancel()
	var result analysis.Report
	var err error
	func() {
		defer func() {
			if recover() != nil {
				err = errors.New("analysis panic")
			}
		}()
		result, err = m.compute(ctx, t.input, func(p int, phase string) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.latest[t.table] != t || ctx.Err() != nil {
				return
			}
			if p > t.view.Progress {
				t.view.Progress = min(99, p)
			}
			t.view.Phase = phase
			t.view.UpdatedAt = time.Now()
		})
	}()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t.input = Snapshot{}
	if m.latest[t.table] != t {
		return
	}
	t.view.UpdatedAt = time.Now()
	if err != nil {
		t.view.Status = "failed"
		t.view.Error = "分析未完成，可重试；记录已保存"
		if errors.Is(err, context.DeadlineExceeded) {
			t.view.Error = "分析超过时间预算，可缩小数据范围后重试"
		}
		t.view.Phase = "分析未完成"
		return
	}
	t.report = &result
	t.view.Status = "done"
	t.view.Progress = 100
	t.view.Phase = "分析完成"
	t.view.HasResult = true
}
