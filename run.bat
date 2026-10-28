@echo off
rem Builds goplay with the giode CLI and runs it. Arguments are passed to
rem the player, e.g.: run.bat C:\Videos\movie.mkv
rem The toolchain is detected automatically; see scripts\windows.ps1.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\windows.ps1" run %*
exit /b %errorlevel%
