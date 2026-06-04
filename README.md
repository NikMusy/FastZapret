# FastZapret

> Высокоскоростной DPI-обход для Windows. Аналог [zapret-discord-youtube](https://github.com/Flowseal/zapret-discord-youtube) — только **быстрее в разы** и со встроенным веб-интерфейсом.

[**Скачать релиз**](https://github.com/NikMusy/FastZapret/releases/latest) · [**Сайт проекта**](https://nikmusy.github.io/FastZapret/) · [**Issues**](https://github.com/NikMusy/FastZapret/issues)

---

## Что разблокирует

| Сервис | Что включает |
|---|---|
| **Discord** | текст + голосовые каналы (UDP) |
| **YouTube** | HTTPS + QUIC, видео без буферизации |
| **Telegram** | MTProto через 443, без прокси |
| **Roblox** | вход + игровой UDP-трафик |

## Почему быстрее zapret

| | zapret | **FastZapret** |
|---|---|---|
| Хэндлов WinDivert | 1 | N = число ядер CPU |
| Пакетов за syscall | 1 | до 64 (`RecvEx`/`SendEx`) |
| Парсер | C, с аллокациями | Go, zero-alloc |
| Checksum | через helper-syscall | свой инлайн (~30 % быстрее) |
| Фильтр в драйвере | широкий | узкий — нерелевантное не покидает kernel |
| Hostname-aware | нет | да, по SNI |
| Веб-UI | нет | встроен на `127.0.0.1:7890` |

## Запуск

1. Скачайте [последний релиз](https://github.com/NikMusy/FastZapret/releases/latest), распакуйте.
2. Запустите `fastzapret-gui.exe` от имени **Администратора**.
3. Откроется веб-панель в браузере. Готово.

Либо как сервис автозапуска:

```cmd
scripts\install_service.bat
```

## Профили

- `fast` — split2 (одна модификация на пакет, минимум накладных)
- `balanced` — fake(TTL=4) + split (по умолчанию)
- `aggressive` — fake + disorder + split (для жёстких DPI)
- `max` — multi-fake + bad-checksum + disorder (когда ничего больше не помогает)

Меняется в веб-UI или в `configs\default.ini`.

## Сборка из исходников

```cmd
go build -ldflags="-s -w -H windowsgui" -o fastzapret-gui.exe .
go build -ldflags="-s -w" -o fastzapret-cli.exe .
```

Нужен Go 1.22+.

## Архитектура

```
divert/        — WinDivert wrapper, N параллельных воркеров с batched I/O
ipparse/       — zero-alloc IP/TCP/UDP/TLS парсер + checksum
strategy/      — Op: TCPFake, TCPSplit, TCPMultiFake, TCPFakeBadChecksum, UDPFake
services/      — per-сервис пайплайны + SNI matching
engine/        — управление воркерами, hot-reload профиля
webui/         — встроенный HTTP UI (HTML + SSE для real-time stats)
config/        — INI-парсер без зависимостей
```

## Лицензия

MIT (мой код) · LGPL/GPL (WinDivert)

## Дисклеймер

Используйте в рамках законодательства вашей страны. Автор не несёт ответственности за способы применения.
