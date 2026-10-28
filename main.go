// goplay is a video player: mpv's playback engine with a Giode interface.
package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/ncruces/zenity"

	"github.com/mehran9675/goplay/internal/app"
)

func main() {
	log.SetFlags(0)
	defer func() {
		if r := recover(); r != nil {
			fatal(fmt.Errorf("%v\n\n%s", r, debug.Stack()))
		}
	}()

	var files, commands []string
	for _, arg := range os.Args[1:] {
		switch {
		case strings.HasPrefix(arg, "--giode-action="):
			// A file manager's "Play with GoPlay": Giode hands the
			// paths after it to the app.
			files = nil
			goto parsed
		case arg == "--version" || arg == "-v":
			fmt.Println("goplay", app.Version)
			return
		case arg == "--help" || arg == "-h":
			fmt.Print(usage)
			return
		case strings.HasPrefix(arg, "--run="):
			for _, c := range strings.Split(strings.TrimPrefix(arg, "--run="), ",") {
				if c = strings.TrimSpace(c); c != "" {
					commands = append(commands, c)
				}
			}
		case strings.HasPrefix(arg, "-psn_"):
			// macOS passes a process serial number when launched from
			// Finder; files arrive as open events instead.
		default:
			files = append(files, arg)
		}
	}
parsed:

	a, err := app.New(files, commands, iconPNG)
	if err != nil {
		fatal(err)
	}
	if err := a.Run(); err != nil {
		fatal(err)
	}
}

//go:embed assets/icon.png
var iconPNG []byte

const usage = `Usage: goplay [--run=command,...] [file, folder or URL ...]

--run runs commands once the window is up, e.g. --run=fullscreen,info.
Command names are listed in Settings > Shortcuts and the command
palette (Ctrl+K / Cmd+K).
`

// fatal reports an unrecoverable error in a dialog (a windowed build has
// no console) as well as on stderr, and exits.
func fatal(err error) {
	log.Print(err)
	zenity.Error(err.Error(), zenity.Title("goplay"))
	os.Exit(1)
}
