# Обновляет lists\ipset-lmu.txt: свежие диапазоны Cloudflare + Hetzner (AS24940).
# Запуск: правый клик -> Run with PowerShell, либо scripts\update_lmu_ipset.bat
$ErrorActionPreference = 'Stop'
$root  = Split-Path -Parent $PSScriptRoot
$out   = Join-Path $root 'lists\ipset-lmu.txt'

Write-Host 'Загружаю диапазоны Cloudflare...'
$cf = (Invoke-WebRequest -UseBasicParsing 'https://www.cloudflare.com/ips-v4').Content -split "`n" |
      ForEach-Object { $_.Trim() } | Where-Object { $_ -match '^\d' }

Write-Host 'Загружаю диапазоны Hetzner (AS24940)...'
$hz = (Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/ipverse/asn-ip/master/as/24940/ipv4-aggregated.txt').Content -split "`n" |
      ForEach-Object { $_.Trim() } | Where-Object { $_ -match '^\d' }

$lines = @()
$lines += '# ipset-lmu.txt — автообновление: Cloudflare + Hetzner (AS24940).'
$lines += '# Используется только в LMU-профиле с мягким desync.'
$lines += ''
$lines += '# --- Cloudflare (api / auth / lobby) ---'
$lines += $cf
$lines += ''
$lines += '# --- Hetzner Online GmbH / AS24940 (game / dedicated servers) ---'
$lines += $hz

Set-Content -Path $out -Value $lines -Encoding UTF8
Write-Host ("Готово: {0} строк -> {1}" -f ($cf.Count + $hz.Count), $out)
