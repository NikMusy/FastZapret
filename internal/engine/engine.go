//go:build windows

// Package engine управляет дочерним процессом winws.exe.
//
// FastZapret сам DPI не обходит — он запускает движок winws.exe (bol-van/zapret)
// с набором стратегий, собранным пакетом winws, следит за процессом, ловит его
// вывод в лог и умеет останавливать/перезапускать.
package engine

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/NikMusy/FastZapret/internal/winws"
)

const createNoWindow = 0x08000000

// Engine управляет одним процессом winws.exe.
type Engine struct {
	mu       sync.Mutex
	binDir   string
	listsDir string
	exe      string
	prof     winws.Profile

	cmd       *exec.Cmd
	running   bool
	startedAt time.Time
	lastCmd   string
	onExit    func(err error)

	logMu  sync.Mutex
	logs   []string
	logCap int
}

// New создаёт движок. binDir — папка с winws.exe и bin-файлами,
// listsDir — папка с хостлистами/ipset.
func New(prof winws.Profile, binDir, listsDir string) *Engine {
	return &Engine{
		binDir:   binDir,
		listsDir: listsDir,
		exe:      filepath.Join(binDir, "winws.exe"),
		prof:     prof,
		logCap:   500,
	}
}

// SetOnExit регистрирует колбэк, вызываемый когда winws неожиданно завершился.
func (e *Engine) SetOnExit(fn func(err error)) {
	e.mu.Lock()
	e.onExit = fn
	e.mu.Unlock()
}

// Start запускает winws.exe с текущим профилем.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startLocked()
}

func (e *Engine) startLocked() error {
	if e.running {
		return nil
	}
	// подчистить возможные зависшие экземпляры, чтобы не конфликтовать за драйвер
	_ = exec.Command("taskkill", "/F", "/IM", "winws.exe").Run()

	args := winws.BuildArgs(e.prof, e.binDir, e.listsDir)
	e.lastCmd = winws.CommandLine(e.exe, args)

	cmd := exec.Command(e.exe, args...)
	cmd.Dir = e.binDir // чтобы нашлись WinDivert.dll и cygwin1.dll
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout // сливаем stderr в тот же поток

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось запустить winws.exe: %w", err)
	}
	e.cmd = cmd
	e.running = true
	e.startedAt = time.Now()
	e.appendLog(fmt.Sprintf("winws.exe запущен (pid %d), стратегия=%s lmu=%v",
		cmd.Process.Pid, e.prof.Strategy, e.prof.LeMans))

	go e.pump(stdout)
	go e.wait(cmd)
	return nil
}

// pump читает вывод winws в кольцевой лог.
func (e *Engine) pump(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		e.appendLog(sc.Text())
	}
}

// wait ждёт завершения процесса и отмечает движок остановленным.
func (e *Engine) wait(cmd *exec.Cmd) {
	err := cmd.Wait()
	e.mu.Lock()
	// если это всё ещё «наш» текущий процесс — фиксируем факт выхода
	stillCurrent := e.cmd == cmd
	if stillCurrent {
		e.running = false
		e.cmd = nil
	}
	cb := e.onExit
	e.mu.Unlock()
	if stillCurrent {
		if err != nil {
			e.appendLog("winws.exe завершился: " + err.Error())
		} else {
			e.appendLog("winws.exe завершился")
		}
		if cb != nil {
			cb(err)
		}
	}
}

// Stop останавливает winws.exe.
func (e *Engine) Stop() error {
	e.mu.Lock()
	cmd := e.cmd
	e.cmd = nil
	e.running = false
	e.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		// жёстко валим дерево процессов winws
		_ = exec.Command("taskkill", "/F", "/T", "/PID",
			strconv.Itoa(cmd.Process.Pid)).Run()
		_ = cmd.Process.Kill()
	}
	// подчистить возможные «осиротевшие» экземпляры
	_ = exec.Command("taskkill", "/F", "/IM", "winws.exe").Run()
	e.appendLog("остановлено")
	return nil
}

// Restart перезапускает движок.
func (e *Engine) Restart() error {
	_ = e.Stop()
	// небольшая пауза, чтобы драйвер/порт освободились
	time.Sleep(400 * time.Millisecond)
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startLocked()
}

// SetProfile меняет профиль и перезапускает движок, если он был запущен.
func (e *Engine) SetProfile(p winws.Profile) error {
	e.mu.Lock()
	wasRunning := e.running
	e.prof = p
	e.mu.Unlock()
	if wasRunning {
		return e.Restart()
	}
	return nil
}

// Profile возвращает текущий профиль.
func (e *Engine) Profile() winws.Profile {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prof
}

// Running сообщает, запущен ли движок.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Status — снимок состояния для UI.
type Status struct {
	Running    bool   `json:"running"`
	PID        int    `json:"pid"`
	UptimeSec  int64  `json:"uptime_sec"`
	Strategy   string `json:"strategy"`
	LeMans     bool   `json:"lemans"`
	LeMansWide bool   `json:"lemans_wide"`
	AllGames   bool   `json:"all_games"`
}

// Status возвращает текущее состояние.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := Status{
		Running:    e.running,
		Strategy:   e.prof.Strategy,
		LeMans:     e.prof.LeMans,
		LeMansWide: e.prof.LeMansWide,
		AllGames:   e.prof.AllGames,
	}
	if e.running && e.cmd != nil && e.cmd.Process != nil {
		st.PID = e.cmd.Process.Pid
		st.UptimeSec = int64(time.Since(e.startedAt).Seconds())
	}
	return st
}

// LastCommand возвращает последнюю собранную командную строку winws.
func (e *Engine) LastCommand() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lastCmd != "" {
		return e.lastCmd
	}
	return winws.CommandLine(e.exe, winws.BuildArgs(e.prof, e.binDir, e.listsDir))
}

// Logs возвращает копию последних строк лога.
func (e *Engine) Logs() []string {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	return append([]string(nil), e.logs...)
}

func (e *Engine) appendLog(line string) {
	e.logMu.Lock()
	if len(e.logs) >= e.logCap {
		e.logs = e.logs[len(e.logs)-e.logCap+1:]
	}
	e.logs = append(e.logs, time.Now().Format("15:04:05 ")+line)
	e.logMu.Unlock()
}
