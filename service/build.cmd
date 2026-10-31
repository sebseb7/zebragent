@echo off
rem Small static exe, no runtime needed (~5 MB). Requires Go: https://go.dev/dl/
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

echo Building ZebraAgent.exe...
go mod tidy
if errorlevel 1 (
    echo [ERROR] go mod tidy failed.
    exit /b 1
)

go build -trimpath -ldflags "-s -w" -o ZebraAgent.exe .
if errorlevel 1 (
    echo [ERROR] go build failed.
    exit /b 1
)

echo [OK] ZebraAgent.exe built successfully.
dir ZebraAgent.exe | findstr "ZebraAgent.exe"
