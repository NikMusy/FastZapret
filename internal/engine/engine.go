// Package engine — управление воркерами и hot-reload профиля.
package engine

import (
	"context"
	"sync"
	"time"

	"github.com/phantom/fastzapret/internal/divert"
	"github.com/phantom/fastzapret/internal/services"
)

// Engine — обвязка вокруг divert.Worker'ов.
// Поддерживает горячую смену профиля без потери трафика:
// аккуратно gracefully завершаем старые хэндлы и поднимаем новые.
type Engine struct {
	mu      sync.Mutex
	prof    services.Profile
	workers int
	dir     string

	queueLen    int
	queueTimeMs int

	stats   *divert.Stats
	handles []*divert.Handle
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
}

// New создаёт движок.
func New(prof services.Profile, workers, queueLen, queueTimeMs int, divertDir string) *Engine {
	return &Engine{
		prof:        prof,
		workers:     workers,
		dir:         divertDir,
		queueLen:    queueLen,
		queueTimeMs: queueTimeMs,
		stats:       &divert.Stats{},
	}
}

// Start поднимает воркеры под текущий профиль.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return nil
	}
	return e.startLocked()
}

func (e *Engine) startLocked() error {
	if err := divert.Load(e.dir); err != nil {
		return err
	}
	router := services.NewRouter(e.prof)
	filter := services.BuildFilter(e.prof)
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.handles = e.handles[:0]
	for i := 0; i < e.workers; i++ {
		h, err := divert.Open(filter, divert.LayerNetwork, 0, divert.FlagDefault)
		if err != nil {
			cancel()
			for _, hh := range e.handles {
				_ = hh.Close()
			}
			e.handles = nil
			return err
		}
		_ = h.SetParam(divert.ParamQueueLength, uint64(e.queueLen))
		_ = h.SetParam(divert.ParamQueueTime, uint64(e.queueTimeMs))
		e.handles = append(e.handles, h)
		w := divert.NewWorker(i, h, router, e.stats)
		e.wg.Add(1)
		go func(w *divert.Worker) {
			defer e.wg.Done()
			_ = w.Run(ctx)
		}(w)
	}
	e.running = true
	return nil
}

// Stop останавливает движок и закрывает хэндлы.
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopLocked()
}

func (e *Engine) stopLocked() error {
	if !e.running {
		return nil
	}
	for _, h := range e.handles {
		_ = h.Shutdown(2)
	}
	if e.cancel != nil {
		e.cancel()
	}
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	for _, h := range e.handles {
		_ = h.Close()
	}
	e.handles = nil
	e.running = false
	return nil
}

// Restart — остановить и снова запустить.
func (e *Engine) Restart() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.stopLocked()
	return e.startLocked()
}

// SetProfile — смена профиля с автоперезапуском.
func (e *Engine) SetProfile(p services.Profile) error {
	e.mu.Lock()
	wasRunning := e.running
	if wasRunning {
		_ = e.stopLocked()
	}
	e.prof = p
	var err error
	if wasRunning {
		err = e.startLocked()
	}
	e.mu.Unlock()
	return err
}

// Profile возвращает текущий профиль.
func (e *Engine) Profile() services.Profile {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prof
}

// Stats возвращает копию статистики.
func (e *Engine) Stats() divert.Stats {
	return divert.Stats{
		RxPackets: e.stats.RxPackets,
		TxPackets: e.stats.TxPackets,
		Modified:  e.stats.Modified,
		Errors:    e.stats.Errors,
	}
}

// Running — true, если воркеры активны.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}
