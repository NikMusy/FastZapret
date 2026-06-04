@echo off
taskkill /IM fastzapret.exe /F 2>nul
sc stop FastZapret 2>nul
echo done.
