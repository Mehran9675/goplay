@echo off
setlocal

rem ---------------------------------------------------------------------------
rem goplay Windows build script
rem   - sets up the MinGW + libmpv toolchain environment
rem   - builds a GUI-subsystem exe (no console window)
rem   - copies the runtime DLLs next to goplay.exe
rem
rem Runtime DLLs:
rem   - libmpv-2.dll    the mpv playback engine
rem   - libEGL.dll      ANGLE EGL loader (GLFW loads it for the EGL context)
rem   - libGLESv2.dll   ANGLE OpenGL ES implementation (Gio loads it)
rem Adjust the paths below if your toolchain lives elsewhere.
rem ---------------------------------------------------------------------------

set "MINGW=C:\Users\Mehra\mingw64\bin"
set "LIBMPV=C:\Users\Mehra\libmpv"
set "ANGLE=C:\Users\Mehra\AppData\Local\Programs\Microsoft VS Code\cfbea10c5f"

set "PATH=%MINGW%;%LIBMPV%;%PATH%"
set "PKG_CONFIG_PATH=%LIBMPV%"
set "CGO_ENABLED=1"
set "CC=gcc"
set "CXX=g++"

echo Building goplay.exe ...
go build -ldflags "-H=windowsgui" -o goplay.exe .
if errorlevel 1 goto :fail

echo Copying runtime DLLs ...
copy /y "%LIBMPV%\libmpv-2.dll" "%~dp0" >nul
if exist "%ANGLE%\libEGL.dll" (
    copy /y "%ANGLE%\libEGL.dll" "%~dp0" >nul
    copy /y "%ANGLE%\libGLESv2.dll" "%~dp0" >nul
) else (
    echo WARNING: ANGLE DLLs not found at "%ANGLE%" - the window will not open.
)

echo.
echo Build complete: %~dp0goplay.exe
goto :eof

:fail
echo.
echo Build FAILED.
exit /b 1
