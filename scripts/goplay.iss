; Windows installer for goplay, built by scripts/package.sh with Inno Setup:
;   ISCC /DVersion=1.2.3 /DArch=x64|arm64 /DSource=<folder> /DOutput=<folder> goplay.iss
; Source holds goplay.exe, its DLLs and goplay.ico.
;
; It installs for the current user without asking for administrator
; rights (or for everyone, if chosen), adds a Start menu entry, puts goplay
; in Explorer's Open With list for video and audio, and adds "Play with
; GoPlay" to the context menu of video and audio files and folders.

#if Arch == "arm64"
  #define Allowed "arm64"
#else
  #define Allowed "x64compatible"
#endif

[Setup]
AppId={{6F1C2A57-3B0E-4C59-9D8A-2E7B9C41F0A3}
AppName=goplay
AppVersion={#Version}
AppVerName=goplay {#Version}
AppPublisher=Mehran Zahiroddini
AppPublisherURL=https://github.com/Mehran9675/goplay
DefaultDirName={autopf}\goplay
DefaultGroupName=goplay
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed={#Allowed}
ArchitecturesInstallIn64BitMode={#Allowed}
ChangesAssociations=yes
SetupIconFile={#Source}\goplay.ico
UninstallDisplayIcon={app}\goplay.exe
UninstallDisplayName=goplay
OutputDir={#Output}
OutputBaseFilename=goplay-{#Version}-windows-{#Arch}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern

[Tasks]
Name: desktopicon; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: contextmenu; Description: "Add ""Play with GoPlay"" to the context menu of videos, music and folders"; GroupDescription: "Explorer:"

[Files]
Source: "{#Source}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs

[Icons]
Name: "{autoprograms}\goplay"; Filename: "{app}\goplay.exe"; IconFilename: "{app}\goplay.ico"
Name: "{autodesktop}\goplay"; Filename: "{app}\goplay.exe"; IconFilename: "{app}\goplay.ico"; Tasks: desktopicon

[Registry]
; Open With: goplay's entry and the file types it lists itself under.
Root: HKA; Subkey: "Software\Classes\Applications\goplay.exe"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "goplay"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Classes\Applications\goplay.exe\DefaultIcon"; ValueType: string; ValueData: "{app}\goplay.ico"
Root: HKA; Subkey: "Software\Classes\Applications\goplay.exe\shell\open\command"; ValueType: string; ValueData: """{app}\goplay.exe"" ""%1"""
#define Types "mkv webm mp4 m4v mov avi ts m2ts mts flv wmv mpg mpeg ogv vob 3gp mp3 flac ogg opus m4a aac wav wma"
#sub OpenWith
  #define T Copy(Types, 1, Pos(" ", Types + " ") - 1)
Root: HKA; Subkey: "Software\Classes\Applications\goplay.exe\SupportedTypes"; ValueType: string; ValueName: ".{#T}"; ValueData: ""
Root: HKA; Subkey: "Software\Classes\.{#T}\OpenWithList\goplay.exe"; ValueType: none; Flags: uninsdeletekey
  #define Types Copy(Types, Pos(" ", Types + " ") + 1, Len(Types))
#endsub
#for {0; Len(Types) > 0; 0} OpenWith

; Play with GoPlay, on video and audio files (whatever app opens them) and
; on folders. On Windows 11 it's under "Show more options".
#sub Verb
Root: HKA; Subkey: "Software\Classes\{#Key}\shell\goplay"; ValueType: string; ValueData: "Play with GoPlay"; Flags: uninsdeletekey; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\{#Key}\shell\goplay"; ValueType: string; ValueName: "Icon"; ValueData: "{app}\goplay.ico"; Tasks: contextmenu
Root: HKA; Subkey: "Software\Classes\{#Key}\shell\goplay\command"; ValueType: string; ValueData: """{app}\goplay.exe"" ""%1"""; Tasks: contextmenu
#endsub
#define Key "SystemFileAssociations\video"
#expr Verb
#define Key "SystemFileAssociations\audio"
#expr Verb
#define Key "Directory"
#expr Verb

[Run]
Filename: "{app}\goplay.exe"; Description: "{cm:LaunchProgram,goplay}"; Flags: nowait postinstall skipifsilent
