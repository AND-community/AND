@echo off
chcp 65001 >nul
echo.
echo  ╔══════════════════════════════════════╗
echo  ║        AND - Sistem Kurulumu         ║
echo  ╚══════════════════════════════════════╝
echo.

set "AND_DIZIN=%~dp0"
:: Son ters bölü işaretini kaldır
if "%AND_DIZIN:~-1%"=="\" set "AND_DIZIN=%AND_DIZIN:~0,-1%"

echo  AND dizini: %AND_DIZIN%
echo.

:: Mevcut kullanıcı PATH'ini oku
for /f "usebackq tokens=2*" %%A in (`reg query "HKCU\Environment" /v PATH 2^>nul`) do set "MEVCUT_PATH=%%B"

:: Zaten eklenmiş mi kontrol et
echo %MEVCUT_PATH% | findstr /i /c:"%AND_DIZIN%" >nul 2>&1
if not errorlevel 1 (
    echo  [TAMAM] AND zaten PATH'te kayitli.
    echo.
    goto bitti
)

:: PATH'e ekle
powershell -NoProfile -Command ^
  "$p = [Environment]::GetEnvironmentVariable('PATH','User'); ^
   [Environment]::SetEnvironmentVariable('PATH', $p + ';' + '%AND_DIZIN%', 'User')"

if errorlevel 1 (
    echo  [HATA] PATH guncellenemedi. Yonetici olarak calistirmayi dene.
    echo.
    pause
    exit /b 1
)

echo  [TAMAM] PATH basariyla guncellendi.
echo.

:bitti
echo  ┌─────────────────────────────────────────┐
echo  │  Kurulum tamamlandi!                    │
echo  │                                         │
echo  │  Yapman gereken:                        │
echo  │   1. Bu pencereyi kapat                 │
echo  │   2. Yeni bir terminal/PowerShell ac    │
echo  │   3.  and  yazarak uygulamayi baslat   │
echo  └─────────────────────────────────────────┘
echo.
pause
