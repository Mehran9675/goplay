// Package libmpv is a minimal cgo binding for the parts of the libmpv client
// API that goplay needs: handle management, options/properties, commands, and
// the OpenGL render API (see render.go).
package libmpv

/*
#cgo pkg-config: mpv
#include <stdlib.h>
#include <mpv/client.h>

// goplay_error_string returns the description for a libmpv status code.
static const char* goplay_error_string(int err) {
	return mpv_error_string(err);
}

// goplay_event_name maps event ids to names for diagnostics.
static const char* goplay_event_name(int id) {
	switch (id) {
	case MPV_EVENT_NONE: return "none";
	case MPV_EVENT_SHUTDOWN: return "shutdown";
	case MPV_EVENT_LOG_MESSAGE: return "log";
	case MPV_EVENT_START_FILE: return "start-file";
	case MPV_EVENT_FILE_LOADED: return "file-loaded";
	case MPV_EVENT_END_FILE: return "end-file";
	case MPV_EVENT_VIDEO_RECONFIG: return "video-reconfig";
	case MPV_EVENT_AUDIO_RECONFIG: return "audio-reconfig";
	case MPV_EVENT_PLAYBACK_RESTART: return "playback-restart";
	case MPV_EVENT_SEEK: return "seek";
	case MPV_EVENT_IDLE: return "idle";
	}
	return "other";
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// Handle wraps an mpv_handle. It is not safe for concurrent use.
type Handle struct {
	h *C.mpv_handle
}

// Create allocates a new mpv handle. The caller must call TerminateDestroy.
func Create() (*Handle, error) {
	h := C.mpv_create()
	if h == nil {
		return nil, errors.New("libmpv: mpv_create failed")
	}
	return &Handle{h: h}, nil
}

// SetOption sets a string option. Must be called before Initialize.
func (m *Handle) SetOption(name, value string) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))
	if e := C.mpv_set_option_string(m.h, cname, cvalue); e < 0 {
		return errorCode(e)
	}
	return nil
}

// Initialize starts the mpv core.
func (m *Handle) Initialize() error {
	if e := C.mpv_initialize(m.h); e < 0 {
		return errorCode(e)
	}
	return nil
}

// Command runs an mpv command (e.g. "loadfile", "seek", "set", "cycle").
func (m *Handle) Command(args ...string) error {
	cargs := make([]*C.char, len(args)+1)
	for i, a := range args {
		cargs[i] = C.CString(a)
	}
	cargs[len(args)] = nil
	defer func() {
		for _, c := range cargs {
			if c != nil {
				C.free(unsafe.Pointer(c))
			}
		}
	}()
	if e := C.mpv_command(m.h, &cargs[0]); e < 0 {
		return errorCode(e)
	}
	return nil
}

// SetDouble sets a numeric property (e.g. "volume", "time-pos").
func (m *Handle) SetDouble(name string, v float64) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cv := C.double(v)
	if e := C.mpv_set_property(m.h, cname, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&cv)); e < 0 {
		return errorCode(e)
	}
	return nil
}

// SetString sets a string property.
func (m *Handle) SetString(name, value string) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))
	if e := C.mpv_set_property_string(m.h, cname, cvalue); e < 0 {
		return errorCode(e)
	}
	return nil
}

// GetDouble reads a numeric property. ok is false if it is unavailable.
func (m *Handle) GetDouble(name string) (v float64, ok bool) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cv C.double
	if e := C.mpv_get_property(m.h, cname, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&cv)); e < 0 {
		return 0, false
	}
	return float64(cv), true
}

// RequestLog enables mpv log messages at level ("fatal".."trace").
func (m *Handle) RequestLog(level string) error {
	c := C.CString(level)
	defer C.free(unsafe.Pointer(c))
	if e := C.mpv_request_log_messages(m.h, c); e < 0 {
		return errorCode(e)
	}
	return nil
}

// WaitEvent polls for one event and returns its name and log text (if any).
func (m *Handle) WaitEvent(timeout float64) (string, string) {
	ev := C.mpv_wait_event(m.h, C.double(timeout))
	name := C.GoString(C.goplay_event_name(C.int(ev.event_id)))
	var logText string
	if ev.event_id == C.MPV_EVENT_LOG_MESSAGE && ev.data != nil {
		lm := (*C.mpv_event_log_message)(ev.data)
		if lm.text != nil {
			logText = C.GoString(lm.text)
		}
	}
	return name, logText
}

// GetBool reads a flag property. ok is false if it is unavailable.
func (m *Handle) GetBool(name string) (v, ok bool) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cv C.int
	if e := C.mpv_get_property(m.h, cname, C.MPV_FORMAT_FLAG, unsafe.Pointer(&cv)); e < 0 {
		return false, false
	}
	return cv != 0, true
}

// TerminateDestroy destroys the mpv handle.
func (m *Handle) TerminateDestroy() {
	if m.h != nil {
		C.mpv_terminate_destroy(m.h)
		m.h = nil
	}
}

func errorCode(e C.int) error {
	msg := C.GoString(C.goplay_error_string(e))
	if msg == "" {
		msg = "unknown error"
	}
	return fmt.Errorf("libmpv: %s", msg)
}
