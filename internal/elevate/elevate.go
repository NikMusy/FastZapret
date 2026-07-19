//go:build windows

// Package elevate — проверка прав администратора и перезапуск с UAC.
// winws.exe требует админ-прав (WinDivert-драйвер), поэтому если приложение
// запущено без них — сами поднимаем запрос UAC и перезапускаемся.
package elevate

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// IsAdmin возвращает true, если процесс запущен с правами администратора.
// Открытие \\.\PHYSICALDRIVE0 удаётся только под админом — надёжный признак.
func IsAdmin() bool {
	f, err := os.Open(`\\.\PHYSICALDRIVE0`)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// RelaunchAsAdmin перезапускает текущий exe через ShellExecute с verb "runas"
// (появляется окно UAC). Возвращает ошибку, если пользователь отказал.
func RelaunchAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	var argPtr *uint16
	if strings.TrimSpace(args) != "" {
		argPtr, _ = syscall.UTF16PtrFromString(args)
	}
	const swNormal = 1

	ret, _, _ := shellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(argPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(swNormal),
	)
	if ret <= 32 { // ShellExecute: <=32 — ошибка (в т.ч. отказ UAC)
		return fmt.Errorf("не удалось поднять права (код %d)", ret)
	}
	return nil
}
