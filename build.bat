@echo off
rem Builds goplay.exe (GUI, no console) and copies the runtime DLLs next to it.
rem The toolchain is detected automatically; see scripts\windows.ps1 for the
rem search order and the MINGW / LIBMPV / ANGLE overrides.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\windows.ps1" build
exit /b %errorlevel%
