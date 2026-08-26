package main

import (
	"log"
	"os"
	"runtime"

	"github.com/mehran9675/goplay-ebiten/internal/app"
)

func main() {
	log.SetFlags(0)
	runtime.LockOSThread()

	var playlist []string
	if len(os.Args) > 1 {
		playlist = []string{os.Args[1]}
	}

	a, err := app.New(playlist)
	if err != nil {
		log.Fatal(err)
	}
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
