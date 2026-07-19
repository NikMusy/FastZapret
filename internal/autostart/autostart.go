//go:build windows

// Package autostart — автозапуск FastZapret при входе в Windows.
//
// Используем Планировщик задач с /rl highest: задача стартует с правами
// администратора без запроса UAC на каждом входе (что было бы с ключом Run).
package autostart

import (
	"os"
	"os/exec"
	"syscall"
)

const taskName = "FastZapret"

func hidden(c *exec.Cmd) *exec.Cmd {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c
}

// IsEnabled — есть ли задача автозапуска.
func IsEnabled() bool {
	err := hidden(exec.Command("schtasks", "/query", "/tn", taskName)).Run()
	return err == nil
}

// Enable создаёт задачу автозапуска для текущего exe.
// Требует прав администратора (для /rl highest).
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// команда задачи: "<exe>" -no-open  (панель не открывать на старте)
	tr := `"` + exe + `" -no-open`
	return hidden(exec.Command("schtasks", "/create",
		"/tn", taskName,
		"/tr", tr,
		"/sc", "onlogon",
		"/rl", "highest",
		"/f",
	)).Run()
}

// Disable удаляет задачу автозапуска.
func Disable() error {
	return hidden(exec.Command("schtasks", "/delete", "/tn", taskName, "/f")).Run()
}

// Toggle переключает автозапуск и возвращает новое состояние.
func Toggle() (bool, error) {
	if IsEnabled() {
		err := Disable()
		return false, err
	}
	err := Enable()
	return err == nil, err
}

// Describe — короткий человекочитаемый статус (для логов).
func Describe() string {
	if IsEnabled() {
		return "автозапуск: включён"
	}
	return "автозапуск: выключен"
}
