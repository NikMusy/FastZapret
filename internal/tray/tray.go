//go:build windows

// Package tray — иконка FastZapret в системном трее с меню управления.
package tray

import (
	_ "embed"
	"time"

	"fyne.io/systray"

	"github.com/NikMusy/FastZapret/internal/autostart"
	"github.com/NikMusy/FastZapret/internal/engine"
	"github.com/NikMusy/FastZapret/internal/webui"
)

//go:embed icon.ico
var iconICO []byte

// Run показывает иконку в трее и блокирует до выхода.
// onQuit вызывается при выборе «Выход» (остановить движок, закрыть панель).
func Run(eng *engine.Engine, url string, onQuit func()) {
	onReady := func() {
		systray.SetIcon(iconICO)
		systray.SetTitle("FastZapret")
		systray.SetTooltip("FastZapret — обход блокировок")

		mStatus := systray.AddMenuItem("Статус: …", "")
		mStatus.Disable()
		mToggle := systray.AddMenuItem("Выключить", "Вкл/выкл обход")
		systray.AddSeparator()
		mLmu := systray.AddMenuItemCheckbox("Le Mans Ultimate", "Профиль для онлайна LMU", eng.Profile().LeMans)
		mOpen := systray.AddMenuItem("Открыть панель", "Веб-панель управления")
		mAuto := systray.AddMenuItemCheckbox("Автозапуск с Windows", "Стартовать при входе", autostart.IsEnabled())
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Выход", "Остановить и закрыть")

		// фоновое обновление статуса и галочек
		go func() {
			t := time.NewTicker(1500 * time.Millisecond)
			defer t.Stop()
			for range t.C {
				if eng.Running() {
					st := eng.Status()
					mStatus.SetTitle("● Работает · " + st.Strategy)
					mToggle.SetTitle("Выключить")
					systray.SetTooltip("FastZapret — обход включён (" + st.Strategy + ")")
				} else {
					mStatus.SetTitle("○ Остановлено")
					mToggle.SetTitle("Включить")
					systray.SetTooltip("FastZapret — обход выключен")
				}
				if eng.Profile().LeMans {
					mLmu.Check()
				} else {
					mLmu.Uncheck()
				}
				if autostart.IsEnabled() {
					mAuto.Check()
				} else {
					mAuto.Uncheck()
				}
			}
		}()

		// обработка кликов
		go func() {
			for {
				select {
				case <-mToggle.ClickedCh:
					if eng.Running() {
						_ = eng.Stop()
					} else {
						_ = eng.Start()
					}
				case <-mLmu.ClickedCh:
					p := eng.Profile()
					p.LeMans = !p.LeMans
					if !p.LeMans {
						p.LeMansWide = false
					}
					_ = eng.SetProfile(p)
				case <-mOpen.ClickedCh:
					webui.OpenInBrowser(url)
				case <-mAuto.ClickedCh:
					_, _ = autostart.Toggle()
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}

	systray.Run(onReady, onQuit)
}

// Quit завершает работу трея (и, как следствие, Run).
func Quit() {
	defer func() { _ = recover() }() // на случай, если трей ещё не запущен
	systray.Quit()
}
