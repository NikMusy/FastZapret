@echo off
REM Удалить сервис FastZapret. Запуск от Администратора.
sc stop FastZapret
sc delete FastZapret
taskkill /F /IM winws.exe 2>nul
echo done.
