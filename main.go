// FastZapret — удобный лаунчер и веб-панель для движка обхода DPI winws.exe
// (bol-van/zapret). Наборы стратегий взяты из проекта Flowseal и дополнены
// профилем под Le Mans Ultimate.
//
// Сам DPI обходит winws.exe; FastZapret собирает для него стратегии,
// запускает/останавливает процесс и даёт локальный веб-UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/NikMusy/FastZapret/internal/config"
	"github.com/NikMusy/FastZapret/internal/elevate"
	"github.com/NikMusy/FastZapret/internal/engine"
	"github.com/NikMusy/FastZapret/internal/webui"
)

var (
	flagConfig   = flag.String("config", "", "путь к INI-конфигу (опц.)")
	flagStrategy = flag.String("strategy", "", "default|alt|alt2|alt3 (override)")
	flagLeMans   = flag.Bool("lmu", false, "включить профиль Le Mans Ultimate")
	flagLeMansW  = flag.Bool("lmu-wide", false, "LMU: ловить широкий диапазон портов (медленнее)")
	flagUI       = flag.String("ui", "", "адрес веб-UI (пусто = как в конфиге)")
	flagNoOpen   = flag.Bool("no-open", false, "не открывать браузер")
	flagPrint    = flag.Bool("print", false, "показать командную строку winws и выйти (ничего не запускает)")
	flagNoStart  = flag.Bool("no-start", false, "не запускать движок автоматически")
	flagNoElev   = flag.Bool("no-elevate", false, "не запрашивать права администратора (UAC)")
)

func main() {
	flag.Parse()

	cfg := config.Defaults()
	if *flagConfig != "" {
		c, err := config.Load(*flagConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
		cfg = c
	}
	if *flagStrategy != "" {
		cfg.Strategy = *flagStrategy
	}
	if *flagLeMans {
		cfg.LeMans = true
	}
	if *flagLeMansW {
		cfg.LeMans = true
		cfg.LeMansWide = true
	}
	if *flagUI != "" {
		cfg.UIAddr = *flagUI
	}
	if *flagNoOpen {
		cfg.OpenUI = false
	}
	if *flagNoStart {
		cfg.Autostart = false
	}

	exe, _ := os.Executable()
	root := filepath.Dir(exe)
	binDir := cfg.BinDir
	if binDir == "" {
		binDir = filepath.Join(root, "bin")
	}
	listsDir := cfg.ListsDir
	if listsDir == "" {
		listsDir = filepath.Join(root, "lists")
	}

	eng := engine.New(cfg.Profile(), binDir, listsDir)

	// --print: только показать команду winws, ничего не запуская.
	if *flagPrint {
		fmt.Println(eng.LastCommand())
		return
	}

	// уже запущен другой экземпляр? — просто откроем его панель.
	if cfg.UIAddr != "" && instanceRunning(cfg.UIAddr) {
		fmt.Println("FastZapret уже запущен, открываю панель:", "http://"+cfg.UIAddr+"/")
		if cfg.OpenUI {
			webui.OpenInBrowser("http://" + cfg.UIAddr + "/")
		}
		return
	}

	// winws требует прав администратора — при необходимости запрашиваем UAC.
	if !*flagNoElev && !elevate.IsAdmin() {
		if err := elevate.RelaunchAsAdmin(); err == nil {
			return // права выданы — работает уже поднятый экземпляр
		}
		fmt.Fprintln(os.Stderr, "внимание: нет прав администратора — winws может не запуститься")
	}

	// проверим, что движок на месте
	if _, err := os.Stat(filepath.Join(binDir, "winws.exe")); err != nil {
		fmt.Fprintln(os.Stderr, "не найден winws.exe в", binDir)
		fmt.Fprintln(os.Stderr, "положите рядом папки bin\\ и lists\\")
		os.Exit(1)
	}

	fmt.Println("FastZapret — лаунчер winws (DPI bypass)")
	fmt.Println("стратегия :", cfg.Strategy)
	fmt.Println("Le Mans   :", onoff(cfg.LeMans))

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nостановка...")
		cancel()
	}()

	if cfg.Autostart {
		if err := eng.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "не удалось запустить движок:", err)
			fmt.Fprintln(os.Stderr, "запустите от имени Администратора")
		}
	}

	if cfg.UIAddr != "" {
		srv := webui.New(eng, cfg.UIAddr)
		addr, err := srv.Serve(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "UI: не удалось открыть", cfg.UIAddr, ":", err)
		} else {
			url := "http://" + addr + "/"
			fmt.Println("веб-UI    :", url)
			if cfg.OpenUI {
				time.Sleep(300 * time.Millisecond)
				webui.OpenInBrowser(url)
			}
		}
	}

	<-ctx.Done()
	_ = eng.Stop()
}

func onoff(b bool) string {
	if b {
		return "вкл"
	}
	return "выкл"
}

// instanceRunning — true, если по адресу UI уже кто-то отвечает.
func instanceRunning(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
