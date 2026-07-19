// Package netcheck — быстрая проверка доступности сервисов.
//
// Меряем чистый TCP-connect (SYN → SYN/ACK) на 443 — это реальный RTT (≈пинг),
// один round-trip, а не полный TLS-хендшейк. Берём лучшее из нескольких попыток,
// чтобы убрать джиттер. Так цифры честные и маленькие.
package netcheck

import (
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

const attempts = 3

// Check проверяет цели параллельно.
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
	best := time.Duration(1<<63 - 1)
	addr := t.Host + ":443"
	var lastErr error
	for i := 0; i < attempts; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			lastErr = err
			continue
		}
		d := time.Since(start)
		_ = conn.Close()
		if d < best {
			best = d
		}
		r.OK = true
	}
	if r.OK {
		r.LatencyMs = best.Milliseconds()
	} else if lastErr != nil {
		r.Error = lastErr.Error()
	}
	return r
}
