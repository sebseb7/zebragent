@echo off
setlocal

echo Clearing ZebraAgent registry entries...

:: Delete current user registry key (HKCU)
reg delete "HKCU\Software\ZebraAgent" /f >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo [OK] Removed HKCU\Software\ZebraAgent
) else (
    echo [INFO] HKCU\Software\ZebraAgent not found.
)

:: Delete machine-wide 64-bit registry key (HKLM)
reg delete "HKLM\SOFTWARE\ZebraAgent" /f >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo [OK] Removed HKLM\SOFTWARE\ZebraAgent
) else (
    echo [INFO] HKLM\SOFTWARE\ZebraAgent not found or requires administrator rights.
)

:: Delete 32-bit WOW64 registry key (if present)
reg delete "HKLM\SOFTWARE\WOW6432Node\ZebraAgent" /f >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo [OK] Removed HKLM\SOFTWARE\WOW6432Node\ZebraAgent
)

echo Done.
