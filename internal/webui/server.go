// Package webui — встроенный HTTP UI на 127.0.0.1.
// HTML/JS встроены в бинарь через embed. Никаких внешних файлов.
package webui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/phantom/fastzapret/internal/divert"
	"github.com/phantom/fastzapret/internal/services"
)

//go:embed index.html
var indexHTML []byte

// Engine — абстракция движка для управления через UI.
type Engine interface {
	Profile() services.Profile
	SetProfile(p services.Profile) error
	Stats() divert.Stats
	Running() bool
	Restart() error
	Stop() error
}

// Server обслуживает локальный UI.
type Server struct {
	eng    Engine
	addr   string
	mu     sync.Mutex
	logs   []string // последние N событий
	logCap int
}

// New создаёт сервер UI.
func New(eng Engine, addr string) *Server {
	if addr == "" {
		addr = "127.0.0.1:7890"
	}
	return &Server{eng: eng, addr: addr, logCap: 200}
}

// AppendLog — добавить событие.
func (s *Server) AppendLog(line string) {
	s.mu.Lock()
	if len(s.logs) >= s.logCap {
		s.logs = s.logs[len(s.logs)-s.logCap+1:]
	}
	s.logs = append(s.logs, time.Now().Format("15:04:05 ")+line)
	s.mu.Unlock()
}

// Serve запускает сервер. Возвращает фактический адрес.
func (s *Server) Serve(ctx context.Context) (string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/profile", s.handleProfile)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/restart", s.handleRestart)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/logs", s.handleLogs)

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return "", err
	}
	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
		_ = ln.Close()
	}()
	go func() {
		_ = srv.Serve(ln)
	}()
	return ln.Addr().String(), nil
}

// OpenInBrowser открывает URL в браузере по умолчанию.
func OpenInBrowser(url string) {
	if runtime.GOOS != "windows" {
		return
	}
	// rundll32 url.dll,FileProtocolHandler — самый надёжный способ на Windows.
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

type stateResp struct {
	Running   bool             `json:"running"`
	Profile   services.Profile `json:"profile"`
	Stats     statsView        `json:"stats"`
	Version   string           `json:"version"`
	StartedAt int64            `json:"started_at"`
}

type statsView struct {
	RX  uint64 `json:"rx"`
	TX  uint64 `json:"tx"`
	Mod uint64 `json:"mod"`
	Err uint64 `json:"err"`
}

var startedAt = time.Now().Unix()
var version = "1.0.0"

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	st := s.eng.Stats()
	resp := stateResp{
		Running: s.eng.Running(),
		Profile: s.eng.Profile(),
		Stats: statsView{
			RX:  atomic.LoadUint64(&st.RxPackets),
			TX:  atomic.LoadUint64(&st.TxPackets),
			Mod: atomic.LoadUint64(&st.Modified),
			Err: atomic.LoadUint64(&st.Errors),
		},
		Version:   version,
		StartedAt: startedAt,
	}
	writeJSON(w, resp)
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var p services.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.eng.SetProfile(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.AppendLog(fmt.Sprintf("профиль обновлён: tune=%s", p.Tune))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	_ = s.eng.Stop()
	s.AppendLog("остановлено")
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if err := s.eng.Restart(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.AppendLog("перезапущено")
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleLogs(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	cp := append([]string(nil), s.logs...)
	s.mu.Unlock()
	writeJSON(w, cp)
}

// handleStream — Server-Sent Events: каждую секунду шлём свежие stats.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream not supported", http.StatusInternalServerError)
		return
	}
	tk := time.NewTicker(1 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tk.C:
			st := s.eng.Stats()
			data, _ := json.Marshal(statsView{
				RX:  atomic.LoadUint64(&st.RxPackets),
				TX:  atomic.LoadUint64(&st.TxPackets),
				Mod: atomic.LoadUint64(&st.Modified),
				Err: atomic.LoadUint64(&st.Errors),
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			fl.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
