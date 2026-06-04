// Package config — простой парсер INI-подобного формата (KEY=VALUE),
// без зависимостей вне stdlib. Никаких TOML-либ, чтобы exe был лёгким.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/phantom/fastzapret/internal/services"
)

// Config — все настройки приложения.
type Config struct {
	Workers      int               // 0 = runtime.NumCPU()
	QueueLen     int               // WinDivert queue (по умолчанию 16384)
	QueueTimeMs  int               // время в очереди (по умолчанию 2000)
	WinDivertDir string            // путь к WinDivert.dll
	Profile      services.Profile
}

// Defaults — настройки по умолчанию.
func Defaults() Config {
	return Config{
		Workers:      0,
		QueueLen:     16384,
		QueueTimeMs:  2000,
		WinDivertDir: ".",
		Profile: services.Profile{
			Discord:  true,
			YouTube:  true,
			Telegram: true,
			Roblox:   true,
			Tune:     "balanced",
		},
	}
}

// Load читает INI-файл и накладывает значения на defaults.
func Load(path string) (Config, error) {
	cfg := Defaults()
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		switch key {
		case "workers":
			cfg.Workers, _ = strconv.Atoi(val)
		case "queue_len":
			cfg.QueueLen, _ = strconv.Atoi(val)
		case "queue_time_ms":
			cfg.QueueTimeMs, _ = strconv.Atoi(val)
		case "windivert_dir":
			cfg.WinDivertDir = val
		case "tune":
			cfg.Profile.Tune = val
		case "discord":
			cfg.Profile.Discord = parseBool(val)
		case "youtube":
			cfg.Profile.YouTube = parseBool(val)
		case "telegram":
			cfg.Profile.Telegram = parseBool(val)
		case "roblox":
			cfg.Profile.Roblox = parseBool(val)
		}
	}
	return cfg, sc.Err()
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}
