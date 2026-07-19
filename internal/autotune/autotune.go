// Package autotune — автоподбор стратегии обхода.
//
// Перебирает варианты стратегий, на каждом перезапускает движок и проверяет
// связь, затем выбирает лучший (больше всего доступных сервисов, меньше пинг).
// Требует запущенного winws — иначе все варианты дают одинаковый результат.
package autotune

import (
	"sync"
	"time"

	"github.com/NikMusy/FastZapret/internal/netcheck"
	"github.com/NikMusy/FastZapret/internal/winws"
)

// Controller — то, что автоподбор дёргает у движка.
type Controller interface {
	Profile() winws.Profile
	SetProfile(winws.Profile) error
	Running() bool
	Start() error
}

// Result — итог по одной стратегии.
type Result struct {
	Strategy  string `json:"strategy"`
	OK        int    `json:"ok"`
	Total     int    `json:"total"`
	LatencyMs int64  `json:"latency_ms"` // суммарный пинг доступных сервисов
}

// State — состояние процесса подбора (для UI).
type State struct {
	Running bool     `json:"running"`
	Current string   `json:"current"`
	Done    bool     `json:"done"`
	Best    string   `json:"best"`
	Results []Result `json:"results"`
}

// Tuner проводит автоподбор.
type Tuner struct {
	ctrl Controller
	mu   sync.Mutex
	st   State

	// тайминги (переопределяются в тестах)
	stabilize time.Duration
	timeout   time.Duration
	tries     int
}

// New создаёт тюнер.
func New(c Controller) *Tuner {
	return &Tuner{
		ctrl:      c,
		stabilize: 1800 * time.Millisecond,
		timeout:   2500 * time.Millisecond,
		tries:     1,
	}
}

// State возвращает копию состояния.
func (t *Tuner) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.st
	s.Results = append([]Result(nil), t.st.Results...)
	return s
}

// Start запускает подбор (если ещё не идёт).
func (t *Tuner) Start() {
	t.mu.Lock()
	if t.st.Running {
		t.mu.Unlock()
		return
	}
	t.st = State{Running: true}
	t.mu.Unlock()
	go t.run()
}

func (t *Tuner) run() {
	base := t.ctrl.Profile()
	if !t.ctrl.Running() {
		_ = t.ctrl.Start()
		time.Sleep(t.stabilize)
	}

	var results []Result
	for _, s := range winws.Strategies {
		t.setCurrent(s)
		p := base
		p.Strategy = s
		_ = t.ctrl.SetProfile(p) // перезапускает winws с новой стратегией
		time.Sleep(t.stabilize)

		res := netcheck.CheckN(netcheck.DefaultTargets, t.timeout, t.tries)
		ok := 0
		var lat int64
		for _, r := range res {
			if r.OK {
				ok++
				lat += r.LatencyMs
			}
		}
		results = append(results, Result{Strategy: s, OK: ok, Total: len(res), LatencyMs: lat})
		t.addResult(results)
	}

	best := pickBest(results)
	p := base
	p.Strategy = best
	_ = t.ctrl.SetProfile(p)
	t.finish(best)
}

// pickBest — больше доступных сервисов, при равенстве — меньше пинг.
// При полном равенстве остаётся первый (default).
func pickBest(rs []Result) string {
	if len(rs) == 0 {
		return "default"
	}
	best := rs[0]
	for _, r := range rs[1:] {
		better := r.OK > best.OK ||
			(r.OK == best.OK && r.OK > 0 && r.LatencyMs < best.LatencyMs)
		if better {
			best = r
		}
	}
	return best.Strategy
}

func (t *Tuner) setCurrent(s string) {
	t.mu.Lock()
	t.st.Current = s
	t.mu.Unlock()
}

func (t *Tuner) addResult(rs []Result) {
	t.mu.Lock()
	t.st.Results = append([]Result(nil), rs...)
	t.mu.Unlock()
}

func (t *Tuner) finish(best string) {
	t.mu.Lock()
	t.st.Running = false
	t.st.Done = true
	t.st.Current = ""
	t.st.Best = best
	t.mu.Unlock()
}
