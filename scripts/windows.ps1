# goplay Windows toolchain detection, build and run.
#
# Called by build.bat and run.bat; not meant to be run directly. It finds
# the MinGW-w64 compiler, pkg-config, libmpv and the ANGLE DLLs, stops with
# an error naming whatever is missing, then builds goplay and copies the
# runtime DLLs next to the exe.
#
# Detection order for each dependency: an environment variable override
# (MINGW, LIBMPV, ANGLE), then PATH, then a search of the usual install
# locations. Set the variable to skip the search, e.g.
#
#   set LIBMPV=D:\sdk\libmpv
#   build.bat

param(
    [Parameter(Mandatory = $true)][ValidateSet('build', 'run')][string]$Mode,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$AppArgs
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$script:problems = @()

function Fail([string]$what, [string]$hint) {
    $script:problems += "  - $what`n      $hint"
}

function Step([string]$msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Found([string]$name, [string]$path) { Write-Host ("    {0,-11} {1}" -f $name, $path) }

# Expand a list of directory patterns (wildcards allowed) into existing
# directories, newest first within each pattern so the latest versioned
# folder (VS Code, Chrome, ...) wins.
function Expand-Dirs([string[]]$patterns) {
    foreach ($p in $patterns) {
        if (-not $p) { continue }
        Get-Item -Path $p -ErrorAction SilentlyContinue |
            Where-Object { $_.PSIsContainer } |
            Sort-Object LastWriteTime -Descending |
            ForEach-Object { $_.FullName }
    }
}

# Searches roots up to depth levels deep for file and returns the
# directories that contain it, newest first.
function Search-File([string]$file, [string[]]$roots, [int]$depth) {
    foreach ($r in (Expand-Dirs $roots)) {
        Get-ChildItem -Path $r -Filter $file -File -Recurse -Depth $depth -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending |
            ForEach-Object { $_.DirectoryName }
    }
}

# Roots for the fallback recursive search. Drive roots are left out on
# purpose: a deep walk of C:\Windows takes minutes.
$userRoots = @("$root\deps", "$root\third_party", $env:USERPROFILE, "$env:LOCALAPPDATA\Programs",
    'C:\tools', 'C:\sdk', 'C:\dev', 'D:\tools', 'D:\sdk', 'D:\dev')

# --- Go -----------------------------------------------------------------
Step 'Looking for Go'
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    $goExe = Expand-Dirs @("$env:ProgramFiles\Go\bin", "$env:USERPROFILE\go\bin", "$env:USERPROFILE\sdk\go*\bin") |
        Where-Object { Test-Path "$_\go.exe" } | Select-Object -First 1
    if ($goExe) { $env:PATH = "$goExe;$env:PATH"; $go = Get-Command go -ErrorAction SilentlyContinue }
}
if ($go) { Found 'go' $go.Source }
else { Fail 'Go toolchain not found' 'Install Go 1.26+ from https://go.dev/dl/ and make sure go.exe is on PATH.' }

# --- MinGW-w64 ----------------------------------------------------------
Step 'Looking for the MinGW-w64 compiler'
$mingwCandidates = @()
if ($env:MINGW) { $mingwCandidates += @($env:MINGW, "$env:MINGW\bin") }
$gccOnPath = Get-Command gcc -ErrorAction SilentlyContinue
if ($gccOnPath) { $mingwCandidates += (Split-Path $gccOnPath.Source) }
$mingwCandidates += Expand-Dirs @("$env:USERPROFILE\mingw64\bin", 'C:\msys64\mingw64\bin', 'C:\msys64\ucrt64\bin',
    'C:\msys64\clang64\bin', 'C:\mingw64\bin', 'C:\MinGW\bin', 'C:\TDM-GCC-64\bin', 'C:\ProgramData\mingw64\mingw64\bin',
    'C:\ProgramData\chocolatey\lib\mingw\tools\install\mingw64\bin', "$env:USERPROFILE\scoop\apps\mingw\current\bin",
    "$env:LOCALAPPDATA\Programs\mingw64\bin", "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\*\mingw64\bin")
$mingw = $mingwCandidates | Where-Object { $_ -and (Test-Path "$_\gcc.exe") } | Select-Object -First 1
if (-not $mingw) {
    $mingw = Search-File 'gcc.exe' ($userRoots + 'C:\msys64') 3 |
        Where-Object { Test-Path "$_\libwinpthread-1.dll" } | Select-Object -First 1
}
$runtimeDlls = @('libgcc_s_seh-1.dll', 'libstdc++-6.dll', 'libwinpthread-1.dll')
if ($env:MINGW -and $mingw -ne $env:MINGW -and $mingw -ne "$env:MINGW\bin") {
    Fail "MINGW is set to '$env:MINGW' but no gcc.exe is there" 'Point MINGW at the MinGW-w64 bin folder (the one containing gcc.exe), or unset it to search automatically.'
}
elseif (-not $mingw) {
    Fail 'MinGW-w64 gcc.exe not found' 'Install MSYS2 (https://www.msys2.org) and run: pacman -S mingw-w64-x86_64-gcc. Or set MINGW to your MinGW bin folder.'
}
else {
    Found 'gcc' "$mingw\gcc.exe"
    foreach ($d in $runtimeDlls) {
        if (-not (Test-Path "$mingw\$d")) { Fail "MinGW runtime $d missing from $mingw" 'Use a complete MinGW-w64 install (x86_64, SEH); the exe needs this DLL at runtime.' }
    }
}

# --- libmpv -------------------------------------------------------------
# Accepts both layouts: a flat SDK folder (libmpv-2.dll, libmpv.dll.a,
# include\mpv\client.h - the shinchiro dev builds) and an MSYS2-style
# prefix (bin\libmpv-2.dll, lib\libmpv.dll.a, include\mpv\client.h).
Step 'Looking for libmpv'
function Get-MpvRoot([string]$dir) {
    if (-not $dir) { return $null }
    foreach ($r in @($dir, (Split-Path -Parent $dir))) {
        if ($r -and (Test-Path "$r\include\mpv\client.h") -and
            ((Test-Path "$r\libmpv-2.dll") -or (Test-Path "$r\bin\libmpv-2.dll"))) { return $r }
    }
    return $null
}
$libmpv = $null
if ($env:LIBMPV) {
    $libmpv = Get-MpvRoot $env:LIBMPV
    if (-not $libmpv) { Fail "LIBMPV is set to '$env:LIBMPV' but it has no libmpv-2.dll + include\mpv\client.h" 'Point LIBMPV at the extracted libmpv dev package, or unset it to search automatically.' }
}
else {
    $mpvDirs = @(Expand-Dirs @("$env:USERPROFILE\libmpv", "$env:USERPROFILE\mpv*", 'C:\libmpv', 'C:\mpv*', "$root\deps\*",
            "$env:LOCALAPPDATA\libmpv", 'C:\msys64\mingw64', 'C:\msys64\ucrt64'))
    if ($mingw) { $mpvDirs += (Split-Path -Parent $mingw) }
    $libmpv = $mpvDirs | ForEach-Object { Get-MpvRoot $_ } | Where-Object { $_ } | Select-Object -First 1
    if (-not $libmpv) {
        $libmpv = Search-File 'libmpv-2.dll' $userRoots 3 | ForEach-Object { Get-MpvRoot $_ } |
            Where-Object { $_ } | Select-Object -First 1
    }
    if (-not $libmpv) {
        Fail 'libmpv development files not found (libmpv-2.dll + include\mpv\client.h)' 'Download "mpv-dev-x86_64-*.7z" from https://github.com/shinchiro/mpv-winbuild-cmake/releases, extract it to %USERPROFILE%\libmpv, or set LIBMPV.'
    }
}
if ($libmpv) {
    $mpvDll = @("$libmpv\libmpv-2.dll", "$libmpv\bin\libmpv-2.dll") | Where-Object { Test-Path $_ } | Select-Object -First 1
    $mpvLibDir = @($libmpv, "$libmpv\lib") | Where-Object { Test-Path "$_\libmpv.dll.a" } | Select-Object -First 1
    Found 'libmpv' $libmpv
    if (-not $mpvLibDir) { Fail "libmpv import library libmpv.dll.a missing under $libmpv" 'Use the libmpv *dev* package; it ships libmpv.dll.a next to libmpv-2.dll.' }
}

# --- pkg-config ---------------------------------------------------------
Step 'Looking for pkg-config'
$pkgConfig = $null
foreach ($name in @('pkg-config.exe', 'pkgconf.exe')) {
    if ($mingw -and (Test-Path "$mingw\$name")) { $pkgConfig = "$mingw\$name"; break }
}
if (-not $pkgConfig) {
    $cmd = Get-Command pkg-config, pkgconf -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($cmd) { $pkgConfig = $cmd.Source }
}
if (-not $pkgConfig) {
    $pkgConfig = Expand-Dirs @('C:\msys64\mingw64\bin', 'C:\msys64\ucrt64\bin', 'C:\ProgramData\chocolatey\bin') |
        ForEach-Object { @("$_\pkg-config.exe", "$_\pkgconf.exe") } | Where-Object { Test-Path $_ } | Select-Object -First 1
}
if ($pkgConfig) { Found 'pkg-config' $pkgConfig }
else { Fail 'pkg-config not found' 'Install it with MSYS2 (pacman -S mingw-w64-x86_64-pkgconf) or Chocolatey (choco install pkgconfiglite).' }

# mpv.pc: use the package's own when present, else write one into a temp
# folder pointing at the detected libmpv, so a bare SDK folder still works.
$pcDir = $null
if ($libmpv -and $mpvLibDir) {
    $pcDir = @($libmpv, "$libmpv\lib\pkgconfig", "$libmpv\pkgconfig") | Where-Object { Test-Path "$_\mpv.pc" } | Select-Object -First 1
    if ($pcDir) {
        # A .pc file with a stale prefix (the folder was moved) would make
        # cgo fail with a confusing missing-header error.
        $prefix = (Select-String -Path "$pcDir\mpv.pc" -Pattern '^prefix=(.*)$' | Select-Object -First 1).Matches.Groups[1].Value
        if ($prefix -and -not (Test-Path ($prefix -replace '/', '\'))) { $pcDir = $null }
    }
    if (-not $pcDir) {
        $pcDir = Join-Path $env:TEMP 'goplay-pkgconfig'
        New-Item -ItemType Directory -Force $pcDir | Out-Null
        $fwd = { param($p) $p -replace '\\', '/' }
        @(
            "prefix=$(& $fwd $libmpv)",
            "libdir=$(& $fwd $mpvLibDir)",
            'includedir=${prefix}/include',
            '',
            'Name: mpv',
            'Description: libmpv - media player client library',
            'Version: 0.0.0',
            'Libs: -L${libdir} -lmpv',
            'Cflags: -I${includedir}'
        ) | Set-Content -Encoding ascii "$pcDir\mpv.pc"
        Write-Host "    (generated $pcDir\mpv.pc)"
    }
}

# --- ANGLE --------------------------------------------------------------
# Gio and libmpv share an OpenGL ES context through ANGLE. Any recent
# Chromium-based app ships a usable libEGL.dll + libGLESv2.dll pair.
Step 'Looking for ANGLE (libEGL.dll, libGLESv2.dll)'
function Test-Angle([string]$d) { $d -and (Test-Path "$d\libEGL.dll") -and (Test-Path "$d\libGLESv2.dll") }
$angle = $null
if ($env:ANGLE) {
    if (Test-Angle $env:ANGLE) { $angle = $env:ANGLE }
    else { Fail "ANGLE is set to '$env:ANGLE' but libEGL.dll/libGLESv2.dll are not both there" 'Point ANGLE at a folder with both DLLs, or unset it to search automatically.' }
}
else {
    $angle = Expand-Dirs @("$root\deps\angle", "$env:USERPROFILE\angle", "$env:LOCALAPPDATA\Programs\Microsoft VS Code\*",
        "$env:LOCALAPPDATA\Programs\Microsoft VS Code", "$env:ProgramFiles\Microsoft VS Code\*", "$env:ProgramFiles\Microsoft VS Code",
        "$env:ProgramFiles\Google\Chrome\Application\*", "${env:ProgramFiles(x86)}\Google\Chrome\Application\*",
        "$env:LOCALAPPDATA\Google\Chrome\Application\*", "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\*",
        "$env:ProgramFiles\Microsoft\Edge\Application\*", "$env:ProgramFiles\BraveSoftware\Brave-Browser\Application\*") |
        Where-Object { Test-Angle $_ } | Select-Object -First 1
    if (-not $angle) {
        Fail 'ANGLE DLLs not found (libEGL.dll + libGLESv2.dll)' 'Install VS Code or Google Chrome (both ship ANGLE), or copy the two DLLs into deps\angle, or set ANGLE.'
    }
}
if ($angle) { Found 'ANGLE' $angle }

# --- Report -------------------------------------------------------------
if ($script:problems.Count -gt 0) {
    Write-Host ''
    Write-Host "ERROR: missing build dependencies:" -ForegroundColor Red
    $script:problems | ForEach-Object { Write-Host $_ -ForegroundColor Red }
    exit 1
}

# --- Build --------------------------------------------------------------
# MinGW goes first on PATH so its runtime DLLs win over other copies (Git
# for Windows ships an incompatible libwinpthread-1.dll).
$env:PATH = "$mingw;$(Split-Path $mpvDll);$angle;$env:PATH"
$env:PKG_CONFIG = $pkgConfig
$env:PKG_CONFIG_PATH = $pcDir
$env:CGO_ENABLED = '1'
$env:CC = 'gcc'
$env:CXX = 'g++'

& $pkgConfig --exists mpv
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: pkg-config cannot resolve mpv using $pcDir\mpv.pc" -ForegroundColor Red
    exit 1
}

if ($Mode -eq 'build') {
    # GUI subsystem: no console window.
    $exe = 'goplay.exe'
    $ldflags = '-H=windowsgui'
}
else {
    # Console subsystem so mpv and Go logs show in the terminal.
    $exe = 'goplay-debug.exe'
    $ldflags = ''
}

Step "Building $exe"
Push-Location $root
try {
    # Windows PowerShell drops empty arguments, so -ldflags is only passed
    # when set.
    $buildArgs = @('build')
    if ($ldflags) { $buildArgs += @('-ldflags', $ldflags) }
    & go @buildArgs -o $exe .
    if ($LASTEXITCODE -ne 0) {
        Write-Host 'ERROR: go build failed.' -ForegroundColor Red
        exit 1
    }

    # The app folder is searched for DLLs before PATH, so copying them
    # next to the exe makes it self-contained.
    Step 'Copying runtime DLLs'
    $dlls = @($mpvDll, "$angle\libEGL.dll", "$angle\libGLESv2.dll") + ($runtimeDlls | ForEach-Object { "$mingw\$_" })
    foreach ($d in $dlls) {
        $dest = Join-Path $root (Split-Path -Leaf $d)
        if ((Test-Path $dest) -and (Get-FileHash $d).Hash -eq (Get-FileHash $dest).Hash) { continue }
        try { Copy-Item -Force $d $root }
        catch { Write-Host "    warning: could not copy $(Split-Path -Leaf $d) (is goplay running?)" -ForegroundColor Yellow }
    }

    if ($Mode -eq 'build') {
        Write-Host ''
        Write-Host "Build complete: $root\$exe" -ForegroundColor Green
        exit 0
    }

    Step "Running $exe $AppArgs"
    & ".\$exe" @AppArgs
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
