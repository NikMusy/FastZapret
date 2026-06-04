// FastZapret — высокоскоростной DPI-обход для Windows.
// Параллельные WinDivert хэндлы (по числу ядер) + batched I/O + zero-alloc
// разбор пакета. Цель — превзойти zapret по пропускной способности
// при тех же DPI-обходных техниках.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/phantom/fastzapret/internal/config"
	"github.com/phantom/fastzapret/internal/engine"
	"github.com/phantom/fastzapret/internal/services"
	"github.com/phantom/fastzapret/internal/webui"
)

var (
	flagConfig = flag.String("config", "", "путь к INI-конфигу (опц.)")
	flagTune   = flag.String("tune", "", "fast|balanced|aggressive|max (override)")
	flagWorks  = flag.Int("workers", 0, "число воркеров (0 = NumCPU)")
	flagUIAddr = flag.String("ui", "127.0.0.1:7890", "адрес локального веб-UI (пусто = отключить)")
	flagOpen   = flag.Bool("open", true, "открыть UI в браузере при старте")
	flagSilent = flag.Bool("silent", false, "не печатать stats в консоль")
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
	if *flagTune != "" {
		cfg.Profile.Tune = *flagTune
	}
	if *flagWorks > 0 {
		cfg.Workers = *flagWorks
	}
	if cfg.Workers == 0 {
		cfg.Workers = runtime.NumCPU()
	}

	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	if cfg.WinDivertDir == "" || cfg.WinDivertDir == "." {
		bin := filepath.Join(dir, "bin")
		if _, err := os.Stat(filepath.Join(bin, "WinDivert.dll")); err == nil {
			cfg.WinDivertDir = bin
		} else {
			cfg.WinDivertDir = dir
		}
	}

	eng := engine.New(cfg.Profile, cfg.Workers, cfg.QueueLen, cfg.QueueTimeMs, cfg.WinDivertDir)
	if err := eng.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "не удалось запустить движок:", err)
		fmt.Fprintln(os.Stderr, "запустите от имени Администратора, проверьте", cfg.WinDivertDir)
		os.Exit(1)
	}

	fmt.Println("FastZapret — DPI bypass")
	fmt.Println("воркеров :", cfg.Workers)
	fmt.Println("режим    :", cfg.Profile.Tune)
	fmt.Println("сервисы  :", servicesList(cfg.Profile))

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nостановка...")
		cancel()
	}()

	if *flagUIAddr != "" {
		srv := webui.New(eng, *flagUIAddr)
		srv.AppendLog("движок запущен")
		addr, err := srv.Serve(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "UI: не удалось открыть", *flagUIAddr, ":", err)
		} else {
			url := "http://" + addr + "/"
			fmt.Println("веб-UI   :", url)
			if *flagOpen {
				time.Sleep(300 * time.Millisecond)
				webui.OpenInBrowser(url)
			}
		}
	}

	if !*flagSilent {
		go statsLoop(ctx, eng)
	}

	<-ctx.Done()
	_ = eng.Stop()
	st := eng.Stats()
	fmt.Printf("итого: RX=%d TX=%d MOD=%d ERR=%d\n",
		st.RxPackets, st.TxPackets, st.Modified, st.Errors)
}

func statsLoop(ctx context.Context, e *engine.Engine) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	var prevRx uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s := e.Stats()
			fmt.Printf("[stats] rx=%d (+%d) tx=%d mod=%d err=%d\n",
				s.RxPackets, s.RxPackets-prevRx, s.TxPackets, s.Modified, s.Errors)
			prevRx = s.RxPackets
		}
	}
}

func servicesList(p services.Profile) string {
	out := ""
	add := func(name string, on bool) {
		if on {
			if out != "" {
				out += " "
			}
			out += name
		}
	}
	add("discord", p.Discord)
	add("youtube", p.YouTube)
	add("telegram", p.Telegram)
	add("roblox", p.Roblox)
	if out == "" {
		out = "<пусто>"
	}
	return out
}
