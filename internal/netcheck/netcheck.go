// Package netcheck — быстрая проверка доступности сервисов (TLS-хендшейк на 443).
// Показывает пользователю, работает ли обход: если хост открывается быстро —
// связь есть, если таймаут — заблокировано/обход не сработал.
package netcheck

import (
	"crypto/tls"
	"net"
	"sync"
	"time"
)

// Result — результат проверки одного сервиса.
type Result struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// Target — что проверяем.
type Target struct {
	Name string
	Host string
}

// DefaultTargets — сервисы по умолчанию.
var DefaultTargets = []Target{
	{"YouTube", "www.youtube.com"},
	{"Discord", "discord.com"},
	{"Le Mans Ultimate", "lemansultimate.com"},
}

// Check проверяет цели параллельно с общим таймаутом на каждую.
func Check(targets []Target, timeout time.Duration) []Result {
	res := make([]Result, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t Target) {
			defer wg.Done()
			res[i] = checkOne(t, timeout)
		}(i, t)
	}
	wg.Wait()
	return res
}

func checkOne(t Target, timeout time.Duration) Result {
	r := Result{Name: t.Name, Host: t.Host}
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(&d, "tcp", t.Host+":443", &tls.Config{
		ServerName:         t.Host,
		InsecureSkipVerify: true, // нас интересует только прохождение хендшейка
	})
	r.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	_ = conn.Close()
	r.OK = true
	return r
}
