package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/ncruces/zenity"

	"github.com/mehran9675/goplay-ebiten/internal/app"
)

func main() {
	log.SetFlags(0)
	runtime.LockOSThread()

	defer func() {
		if r := recover(); r != nil {
			fatal(fmt.Errorf("%v\n\n%s", r, debug.Stack()))
		}
	}()

	var playlist []string
	if len(os.Args) > 1 {
		playlist = []string{os.Args[1]}
	}

	a, err := app.New(playlist)
	if err != nil {
		fatal(err)
	}
	if err := a.Run(); err != nil {
		fatal(err)
	}
}

// fatal reports an unrecoverable error via a popup dialog (in addition to
// stderr, which a windowed build has no visible console for) and exits.
func fatal(err error) {
	log.Print(err)
	zenity.Error(err.Error(), zenity.Title("Fatal Error"))
	os.Exit(1)
}
