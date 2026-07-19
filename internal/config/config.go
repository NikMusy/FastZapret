// Package config — простой парсер INI-подобного формата (KEY=VALUE),
// без зависимостей вне stdlib.
package config

import (
	"bufio"
	"os"
	"strings"

	"github.com/NikMusy/FastZapret/internal/winws"
)

// Config — все настройки приложения.
type Config struct {
	Strategy   string // default | alt | alt2 | alt3
	LeMans     bool   // включить профиль Le Mans Ultimate
	LeMansWide bool   // ловить широкий диапазон портов LMU (медленнее, но шире)
	UIAddr     string // адрес веб-UI (пусто = отключить)
	OpenUI     bool   // открывать браузер при старте
	Autostart  bool   // запускать движок сразу при старте приложения
	BinDir     string // переопределение папки bin (пусто = авто)
	ListsDir   string // переопределение папки lists (пусто = авто)
}

// Defaults — настройки по умолчанию.
func Defaults() Config {
	return Config{
		Strategy:  "default",
		LeMans:    false,
		UIAddr:    "127.0.0.1:7890",
		OpenUI:    true,
		Autostart: true,
	}
}

// Profile строит winws.Profile из конфига.
func (c Config) Profile() winws.Profile {
	s := c.Strategy
	if s == "" {
		s = "default"
	}
	return winws.Profile{Strategy: s, LeMans: c.LeMans, LeMansWide: c.LeMansWide}
}

// Load читает INI-файл поверх defaults.
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
		case "strategy":
			cfg.Strategy = strings.ToLower(val)
		case "lemans", "le_mans", "lmu":
			cfg.LeMans = parseBool(val)
		case "lemans_wide", "lmu_wide":
			cfg.LeMansWide = parseBool(val)
		case "ui", "ui_addr":
			cfg.UIAddr = val
		case "open_ui", "open":
			cfg.OpenUI = parseBool(val)
		case "autostart":
			cfg.Autostart = parseBool(val)
		case "bin_dir":
			cfg.BinDir = val
		case "lists_dir":
			cfg.ListsDir = val
		}
	}
	return cfg, sc.Err()
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}
