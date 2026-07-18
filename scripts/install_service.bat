@echo off
REM Установить FastZapret как Windows-сервис автозапуска. Запуск от Администратора.
setlocal
set ROOT=%~dp0..
sc create FastZapret binPath= "\"%ROOT%\fastzapret-cli.exe\" -config \"%ROOT%\configs\default.ini\" -no-open" start= auto DisplayName= "FastZapret DPI Bypass"
sc description FastZapret "DPI bypass launcher (winws engine): Discord, YouTube, Telegram, Le Mans Ultimate"
sc start FastZapret
echo.
echo Сервис установлен. Для профиля Le Mans Ultimate используйте configs\lmu.ini
endlocal
