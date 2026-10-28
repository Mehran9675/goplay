@echo off
rem Builds goplay-debug.exe (console, shows logs) and runs it. Arguments are
rem passed to the player, e.g.: run.bat C:\Videos\movie.mkv
rem The toolchain is detected automatically; see scripts\windows.ps1.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\windows.ps1" run %*
exit /b %errorlevel%
