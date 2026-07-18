@echo off
REM Остановить FastZapret и движок winws.
taskkill /F /IM fastzapret-gui.exe 2>nul
taskkill /F /IM fastzapret-cli.exe 2>nul
taskkill /F /IM winws.exe 2>nul
sc stop FastZapret 2>nul
echo done.
