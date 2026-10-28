@echo off
rem Builds dist\windows\goplay.exe with the giode CLI, libmpv and ANGLE beside it.
rem The toolchain is detected automatically; see scripts\windows.ps1 for the
rem search order and the MINGW / LIBMPV overrides.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\windows.ps1" build
exit /b %errorlevel%
