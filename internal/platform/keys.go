package platform

import (
	"io"
	"runtime"
	"strings"

	"gioui.org/io/key"
	"gioui.org/io/transfer"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// modifiers converts GLFW modifier bits to Gio's. On macOS the Command
// key (GLFW's Super) is Gio's ModCommand, so key.ModShortcut means
// Command there and Ctrl elsewhere.
func modifiers(m glfw.ModifierKey) key.Modifiers {
	var out key.Modifiers
	if m&glfw.ModControl != 0 {
		out |= key.ModCtrl
	}
	if m&glfw.ModShift != 0 {
		out |= key.ModShift
	}
	if m&glfw.ModAlt != 0 {
		out |= key.ModAlt
	}
	if m&glfw.ModSuper != 0 {
		if runtime.GOOS == "darwin" {
			out |= key.ModCommand
		} else {
			out |= key.ModSuper
		}
	}
	return out
}

var namedKeys = map[glfw.Key]key.Name{
	glfw.KeySpace:        key.NameSpace,
	glfw.KeyEscape:       key.NameEscape,
	glfw.KeyEnter:        key.NameReturn,
	glfw.KeyKPEnter:      key.NameEnter,
	glfw.KeyTab:          key.NameTab,
	glfw.KeyBackspace:    key.NameDeleteBackward,
	glfw.KeyDelete:       key.NameDeleteForward,
	glfw.KeyLeft:         key.NameLeftArrow,
	glfw.KeyRight:        key.NameRightArrow,
	glfw.KeyUp:           key.NameUpArrow,
	glfw.KeyDown:         key.NameDownArrow,
	glfw.KeyHome:         key.NameHome,
	glfw.KeyEnd:          key.NameEnd,
	glfw.KeyPageUp:       key.NamePageUp,
	glfw.KeyPageDown:     key.NamePageDown,
	glfw.KeyF1:           key.NameF1,
	glfw.KeyF2:           key.NameF2,
	glfw.KeyF3:           key.NameF3,
	glfw.KeyF4:           key.NameF4,
	glfw.KeyF5:           key.NameF5,
	glfw.KeyF6:           key.NameF6,
	glfw.KeyF7:           key.NameF7,
	glfw.KeyF8:           key.NameF8,
	glfw.KeyF9:           key.NameF9,
	glfw.KeyF10:          key.NameF10,
	glfw.KeyF11:          key.NameF11,
	glfw.KeyF12:          key.NameF12,
	glfw.KeyComma:        ",",
	glfw.KeyPeriod:       ".",
	glfw.KeySlash:        "/",
	glfw.KeySemicolon:    ";",
	glfw.KeyApostrophe:   "'",
	glfw.KeyLeftBracket:  "[",
	glfw.KeyRightBracket: "]",
	glfw.KeyBackslash:    "\\",
	glfw.KeyMinus:        "-",
	glfw.KeyEqual:        "=",
	glfw.KeyGraveAccent:  "`",
	glfw.KeyKPAdd:        "+",
	glfw.KeyKPSubtract:   "-",
	glfw.KeyKPMultiply:   "*",
	glfw.KeyKPDivide:     "/",
}

// keyName maps a GLFW key to Gio's name for it: letters and digits as
// themselves (letters upper case), symbols by the character on a US
// layout, the rest by Gio's names. Letter keys follow the active
// keyboard layout.
func keyName(k glfw.Key) (key.Name, bool) {
	if n, ok := namedKeys[k]; ok {
		return n, true
	}
	switch {
	case k >= glfw.KeyA && k <= glfw.KeyZ, k >= glfw.Key0 && k <= glfw.Key9:
		if name := glfw.GetKeyName(k, 0); name != "" {
			return key.Name(strings.ToUpper(name)), true
		}
		return key.Name(rune(k)), true
	case k >= glfw.KeyKP0 && k <= glfw.KeyKP9:
		return key.Name(rune('0' + k - glfw.KeyKP0)), true
	}
	return "", false
}

// clipboardEvent delivers text to the editor that asked for the
// clipboard.
func clipboardEvent(text string) transfer.DataEvent {
	return transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader(text)) },
	}
}
