package autotune

import (
	"sync"
	"testing"
	"time"

	"github.com/NikMusy/FastZapret/internal/winws"
)

// fakeCtrl — движок-заглушка, НЕ запускает winws.
type fakeCtrl struct {
	mu       sync.Mutex
	prof     winws.Profile
	running  bool
	setOrder []string
}

func (f *fakeCtrl) Profile() winws.Profile {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prof
}
func (f *fakeCtrl) SetProfile(p winws.Profile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prof = p
	f.setOrder = append(f.setOrder, p.Strategy)
	return nil
}
func (f *fakeCtrl) Running() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}
func (f *fakeCtrl) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = true
	return nil
}

func TestPickBest(t *testing.T) {
	cases := []struct {
		name string
		in   []Result
		want string
	}{
		{"more_ok_wins", []Result{{"default", 1, 3, 40}, {"alt", 3, 3, 300}}, "alt"},
		{"tie_lower_latency", []Result{{"default", 2, 3, 200}, {"alt", 2, 3, 80}}, "alt"},
		{"full_tie_keeps_default", []Result{{"default", 3, 3, 40}, {"alt", 3, 3, 40}}, "default"},
		{"all_zero_keeps_default", []Result{{"default", 0, 3, 0}, {"alt", 0, 3, 0}}, "default"},
		{"empty", nil, "default"},
	}
	for _, c := range cases {
		if got := pickBest(c.in); got != c.want {
			t.Errorf("%s: pickBest = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRun(t *testing.T) {
	f := &fakeCtrl{prof: winws.Profile{Strategy: "default", LeMans: true}}
	tn := New(f)
	tn.stabilize = 5 * time.Millisecond // ускоряем
	tn.timeout = 1500 * time.Millisecond
	tn.tries = 1

	tn.Start()

	// ждём завершения (реальные сетевые проверки могут занять несколько секунд)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if tn.State().Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("autotune не завершился за 30с")
		}
		time.Sleep(100 * time.Millisecond)
	}

	st := tn.State()
	if len(st.Results) != len(winws.Strategies) {
		t.Fatalf("ожидалось %d результатов, получено %d", len(winws.Strategies), len(st.Results))
	}
	if st.Best == "" {
		t.Fatal("Best не выбран")
	}
	// финальный SetProfile должен установить выбранную стратегию
	if got := f.Profile().Strategy; got != st.Best {
		t.Errorf("итоговая стратегия %q != Best %q", got, st.Best)
	}
	// базовые поля профиля должны сохраниться (LeMans не сбрасывается)
	if !f.Profile().LeMans {
		t.Error("LeMans потерялся при подборе")
	}
	// winws должен был «запуститься» (Running=true)
	if !f.Running() {
		t.Error("движок не был запущен для теста стратегий")
	}
}
