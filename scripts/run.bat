@echo off
REM Запуск FastZapret с веб-UI (нужны права Администратора)
pushd "%~dp0\.."
fastzapret-gui.exe -config configs\default.ini
popd
