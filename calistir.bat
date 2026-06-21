@echo off
cd /d "%~dp0"
go build -o and.exe ./cmd/and
if %errorlevel% neq 0 (
    echo Derleme hatasi!
    pause
    exit /b 1
)
and.exe
