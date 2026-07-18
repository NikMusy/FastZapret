@echo off
REM Запуск с консолью и логами winws (для отладки). От Администратора.
pushd "%~dp0\.."
fastzapret-cli.exe -config configs\default.ini
pause
popd
