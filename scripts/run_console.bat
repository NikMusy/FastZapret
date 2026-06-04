@echo off
REM Запуск с консолью (для отладки)
pushd "%~dp0\.."
fastzapret-cli.exe -config configs\default.ini
pause
popd
