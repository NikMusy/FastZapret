@echo off
REM Обновить диапазоны Le Mans Ultimate (Cloudflare + Hetzner) в lists\ipset-lmu.txt
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0update_lmu_ipset.ps1"
pause
