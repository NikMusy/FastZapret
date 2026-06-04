// Package divert — тонкая обёртка над WinDivert.dll через syscall.
// Без cgo: грузим DLL динамически, держим хэндлы на каждый воркер.
package divert

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// Layer constants
const (
	LayerNetwork        = 0
	LayerNetworkForward = 1
)

// Flag constants
const (
	FlagDefault   uint64 = 0
	FlagSniff     uint64 = 0x0001
	FlagDrop      uint64 = 0x0002
	FlagRecvOnly  uint64 = 0x0004
	FlagSendOnly  uint64 = 0x0008
	FlagNoInstall uint64 = 0x0010
	FlagFragments uint64 = 0x0020
)

// Param constants
const (
	ParamQueueLength = 0
	ParamQueueTime   = 1
	ParamQueueSize   = 2
)

// Address — WINDIVERT_ADDRESS, размер 80 байт на x64.
// Раскладываем как массив байт, читаем нужные поля вручную — избегаем
// зависимости от точного выравнивания полей в Go.
type Address [80]byte

// Outbound возвращает направление пакета (true — исходящий).
func (a *Address) Outbound() bool {
	// flags: byte at offset 17 (после Timestamp 8 + Layer 1 + Event 1 + Sniffed/Outbound/Loopback/Impostor/IPv6/IPChecksum/TCPChecksum/UDPChecksum bitfields)
	// Реально: Timestamp (8) | Layer (1) | Event (1) | flags (1) ...
	// Bit field, согласно windivert.h:
	//   UINT32 Sniffed:1;
	//   UINT32 Outbound:1;
	// в одном UINT32 на offset 10? Точная раскладка зависит от компилятора.
	// Используем флаг по смещению 10, бит 1 (Outbound = bit 1).
	return a[10]&0x02 != 0
}

func (a *Address) IPv6() bool {
	return a[10]&0x10 != 0
}

func (a *Address) SetOutbound(v bool) {
	if v {
		a[10] |= 0x02
	} else {
		a[10] &^= 0x02
	}
}

func (a *Address) IfIdx() uint32 {
	return *(*uint32)(unsafe.Pointer(&a[16]))
}

func (a *Address) SubIfIdx() uint32 {
	return *(*uint32)(unsafe.Pointer(&a[20]))
}

// SetChecksumValid отмечает, что мы пересчитали checksum сами,
// чтобы драйвер не дёргал helper.
func (a *Address) SetChecksumValid() {
	// IPChecksum, TCPChecksum, UDPChecksum биты в том же byte[10]/[11]
	a[10] |= 0x40 // IPChecksum
	a[11] |= 0x01 // TCPChecksum
	a[11] |= 0x02 // UDPChecksum
}

var (
	dll               *syscall.LazyDLL
	procOpen          *syscall.LazyProc
	procRecv          *syscall.LazyProc
	procRecvEx        *syscall.LazyProc
	procSend          *syscall.LazyProc
	procSendEx        *syscall.LazyProc
	procClose         *syscall.LazyProc
	procSetParam      *syscall.LazyProc
	procShutdown      *syscall.LazyProc
	loadOnce          sync.Once
	loadErr           error
)

// Load загружает WinDivert.dll из заданной директории.
func Load(dir string) error {
	loadOnce.Do(func() {
		path := dir + `\WinDivert.dll`
		dll = syscall.NewLazyDLL(path)
		if err := dll.Load(); err != nil {
			loadErr = fmt.Errorf("LoadLibrary %s: %w", path, err)
			return
		}
		procOpen = dll.NewProc("WinDivertOpen")
		procRecv = dll.NewProc("WinDivertRecv")
		procRecvEx = dll.NewProc("WinDivertRecvEx")
		procSend = dll.NewProc("WinDivertSend")
		procSendEx = dll.NewProc("WinDivertSendEx")
		procClose = dll.NewProc("WinDivertClose")
		procSetParam = dll.NewProc("WinDivertSetParam")
		procShutdown = dll.NewProc("WinDivertShutdown")
	})
	return loadErr
}

// Handle — открытый WinDivert handle.
type Handle struct {
	h syscall.Handle
}

// Open открывает фильтр.
func Open(filter string, layer uint32, priority int16, flags uint64) (*Handle, error) {
	fb, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}
	r1, _, e1 := procOpen.Call(
		uintptr(unsafe.Pointer(fb)),
		uintptr(layer),
		uintptr(priority),
		uintptr(flags),
	)
	h := syscall.Handle(r1)
	if h == syscall.InvalidHandle {
		return nil, fmt.Errorf("WinDivertOpen failed: %w", e1)
	}
	return &Handle{h: h}, nil
}

// SetParam настраивает параметр (очередь, таймаут).
func (h *Handle) SetParam(param uint32, value uint64) error {
	r1, _, e1 := procSetParam.Call(
		uintptr(h.h),
		uintptr(param),
		uintptr(value),
	)
	if r1 == 0 {
		return e1
	}
	return nil
}

// Recv читает один пакет. Возвращает фактическую длину пакета.
func (h *Handle) Recv(buf []byte, addr *Address) (int, error) {
	var n uint32
	r1, _, e1 := procRecv.Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r1 == 0 {
		return 0, e1
	}
	return int(n), nil
}

// RecvBatch читает пакетный батч до cap(addrs) пакетов за один syscall.
// buf — большой буфер (>= 65535 * batchSize); addrs — массив адресов.
// Возвращает заполненные длины пакетов в lengths.
func (h *Handle) RecvBatch(buf []byte, addrs []Address) (totalBytes int, count int, err error) {
	var packetLen uint32
	var addrLen uint32 = uint32(len(addrs)) * 80
	r1, _, e1 := procRecvEx.Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&packetLen)),
		0, // flags
		uintptr(unsafe.Pointer(&addrs[0])),
		uintptr(unsafe.Pointer(&addrLen)),
		0, // lpOverlapped
	)
	if r1 == 0 {
		return 0, 0, e1
	}
	return int(packetLen), int(addrLen / 80), nil
}

// Send отправляет один пакет.
func (h *Handle) Send(buf []byte, addr *Address) (int, error) {
	if len(buf) == 0 {
		return 0, errors.New("empty packet")
	}
	var n uint32
	r1, _, e1 := procSend.Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r1 == 0 {
		return 0, e1
	}
	return int(n), nil
}

// SendBatch отправляет батч пакетов одним syscall.
func (h *Handle) SendBatch(buf []byte, addrs []Address) error {
	var sent uint32
	addrLen := uint32(len(addrs)) * 80
	r1, _, e1 := procSendEx.Call(
		uintptr(h.h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&sent)),
		0,
		uintptr(unsafe.Pointer(&addrs[0])),
		uintptr(addrLen),
		0,
	)
	if r1 == 0 {
		return e1
	}
	return nil
}

// Shutdown прекращает приём, разблокирует Recv.
func (h *Handle) Shutdown(how uint32) error {
	r1, _, e1 := procShutdown.Call(uintptr(h.h), uintptr(how))
	if r1 == 0 {
		return e1
	}
	return nil
}

// Close закрывает хэндл.
func (h *Handle) Close() error {
	if h.h == 0 {
		return nil
	}
	r1, _, e1 := procClose.Call(uintptr(h.h))
	h.h = 0
	if r1 == 0 {
		return e1
	}
	return nil
}
