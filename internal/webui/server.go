// Package webui — встроенный HTTP UI на 127.0.0.1.
// HTML/JS встроены в бинарь через embed. Никаких внешних файлов.
package webui

import (
	"context"
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/NikMusy/FastZapret/internal/autostart"
	"github.com/NikMusy/FastZapret/internal/engine"
	"github.com/NikMusy/FastZapret/internal/netcheck"
	"github.com/NikMusy/FastZapret/internal/winws"
)

//go:embed index.html
var indexHTML []byte

// Engine — абстракция движка для управления через UI.
type Engine interface {
	Profile() winws.Profile
	SetProfile(p winws.Profile) error
	Status() engine.Status
	Running() bool
	Start() error
	Stop() error
	Restart() error
	Logs() []string
	LastCommand() string
}

// Server обслуживает локальный UI.
type Server struct {
	eng  Engine
	addr string
}

// New создаёт сервер UI.
func New(eng Engine, addr string) *Server {
	if addr == "" {
		addr = "127.0.0.1:7890"
	}
	return &Server{eng: eng, addr: addr}
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
	mux.HandleFunc("/api/start", s.handleStart)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/restart", s.handleRestart)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/check", s.handleCheck)
	mux.HandleFunc("/api/autostart", s.handleAutostart)

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
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), nil
}

// OpenInBrowser открывает URL в браузере по умолчанию.
func OpenInBrowser(url string) {
	if runtime.GOOS != "windows" {
		return
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

type stateResp struct {
	Status      engine.Status `json:"status"`
	Strategies  []string      `json:"strategies"`
	LastCommand string        `json:"last_command"`
	Autostart   bool          `json:"autostart"`
	Version     string        `json:"version"`
}

var version = "2.2.0"

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, stateResp{
		Status:      s.eng.Status(),
		Strategies:  winws.Strategies,
		LastCommand: s.eng.LastCommand(),
		Autostart:   autostart.IsEnabled(),
		Version:     version,
	})
}

func (s *Server) handleAutostart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	on, err := autostart.Toggle()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "autostart": on})
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var p winws.Profile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if p.Strategy == "" {
		p.Strategy = "default"
	}
	if err := s.eng.SetProfile(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if err := s.eng.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	_ = s.eng.Stop()
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
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleLogs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.eng.Logs())
}

// handleCheck — проверка доступности сервисов (работает ли обход).
func (s *Server) handleCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, netcheck.Check(netcheck.DefaultTargets, 4*time.Second))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
