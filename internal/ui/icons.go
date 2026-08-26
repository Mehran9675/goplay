package ui

import (
	"golang.org/x/exp/shiny/materialdesign/icons"

	"gioui.org/widget"
)

// iconSet holds the Material Design icons used by the control bar and menu.
// widget.Icon rasterizes IconVG directly, so no icon font is needed.
type iconSet struct {
	play          *widget.Icon
	pause         *widget.Icon
	stop          *widget.Icon
	next          *widget.Icon
	openFile      *widget.Icon
	openFolder    *widget.Icon
	volumeUp      *widget.Icon
	volumeOff     *widget.Icon
	fullscreen    *widget.Icon
	fullscreenOut *widget.Icon
}

func newIcons() iconSet {
	return iconSet{
		play:          mustIcon(widget.NewIcon(icons.AVPlayArrow)),
		pause:         mustIcon(widget.NewIcon(icons.AVPause)),
		stop:          mustIcon(widget.NewIcon(icons.AVStop)),
		next:          mustIcon(widget.NewIcon(icons.AVSkipNext)),
		openFile:      mustIcon(widget.NewIcon(icons.EditorInsertDriveFile)),
		openFolder:    mustIcon(widget.NewIcon(icons.FileFolderOpen)),
		volumeUp:      mustIcon(widget.NewIcon(icons.AVVolumeUp)),
		volumeOff:     mustIcon(widget.NewIcon(icons.AVVolumeOff)),
		fullscreen:    mustIcon(widget.NewIcon(icons.NavigationFullscreen)),
		fullscreenOut: mustIcon(widget.NewIcon(icons.NavigationFullscreenExit)),
	}
}

func mustIcon(ic *widget.Icon, err error) *widget.Icon {
	if err != nil {
		panic(err)
	}
	return ic
}
