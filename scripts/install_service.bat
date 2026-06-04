@echo off
REM Установить FastZapret как Windows-сервис. Запуск от Администратора.
setlocal
set ROOT=%~dp0..
sc create FastZapret binPath= "\"%ROOT%\fastzapret-cli.exe\" -silent -ui 127.0.0.1:7890 -config \"%ROOT%\configs\default.ini\"" start= auto DisplayName= "FastZapret DPI Bypass"
sc description FastZapret "High-performance DPI bypass: Discord, YouTube, Telegram, Roblox"
sc start FastZapret
endlocal
