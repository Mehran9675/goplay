// Package mpv is a cgo binding for the parts of libmpv's client API that
// goplay needs: creating and configuring the core, commands, properties
// (read, written and observed), events and a wakeup callback. The render
// API is in render.go.
//
// The client API is thread-safe; Handle adds no locking of its own.
package mpv

/*
#cgo pkg-config: mpv
#include <stdlib.h>
#include <stdint.h>
#include <mpv/client.h>

extern void goplayMpvWakeup(uintptr_t id);

static void goplay_wakeup_cb(void* ctx) {
	goplayMpvWakeup((uintptr_t)ctx);
}

static void goplay_set_wakeup(mpv_handle* h, uintptr_t id) {
	mpv_set_wakeup_callback(h, goplay_wakeup_cb, (void*)id);
}

// goplay_event flattens an mpv_event so Go never dereferences mpv's
// event structs. Strings point into mpv's memory and stay valid until
// the next mpv_wait_event call; Go copies them right away.
typedef struct {
	int id;
	int error;
	uint64_t reply;
	const char* name;
	int format;
	double dbl;
	int64_t i64;
	int flag;
	const char* str;
	const char* log_prefix;
	const char* log_level;
	const char* log_text;
	int end_reason;
	int end_error;
} goplay_event;

static goplay_event goplay_wait(mpv_handle* h, double timeout) {
	goplay_event out = {0};
	mpv_event* ev = mpv_wait_event(h, timeout);
	out.id = ev->event_id;
	out.error = ev->error;
	out.reply = ev->reply_userdata;
	switch (ev->event_id) {
	case MPV_EVENT_PROPERTY_CHANGE: {
		mpv_event_property* p = (mpv_event_property*)ev->data;
		out.name = p->name;
		out.format = p->format;
		if (p->data == NULL) {
			out.format = MPV_FORMAT_NONE;
			break;
		}
		switch (p->format) {
		case MPV_FORMAT_DOUBLE: out.dbl = *(double*)p->data; break;
		case MPV_FORMAT_INT64: out.i64 = *(int64_t*)p->data; break;
		case MPV_FORMAT_FLAG: out.flag = *(int*)p->data; break;
		case MPV_FORMAT_STRING: out.str = *(char**)p->data; break;
		default: break;
		}
		break;
	}
	case MPV_EVENT_LOG_MESSAGE: {
		mpv_event_log_message* m = (mpv_event_log_message*)ev->data;
		out.log_prefix = m->prefix;
		out.log_level = m->level;
		out.log_text = m->text;
		break;
	}
	case MPV_EVENT_END_FILE: {
		mpv_event_end_file* e = (mpv_event_end_file*)ev->data;
		out.end_reason = e->reason;
		out.end_error = e->error;
		break;
	}
	default:
		break;
	}
	return out;
}

// goplay_command runs a command given as a NULL-terminated argv built
// in C memory.
static int goplay_command(mpv_handle* h, char** argv) {
	return mpv_command(h, (const char**)argv);
}

static int goplay_command_async(mpv_handle* h, uint64_t reply, char** argv) {
	return mpv_command_async(h, reply, (const char**)argv);
}

static char** goplay_argv(int n) {
	return (char**)calloc(n + 1, sizeof(char*));
}

static void goplay_argv_set(char** argv, int i, char* s) {
	argv[i] = s;
}

static void goplay_argv_free(char** argv, int n) {
	for (int i = 0; i < n; i++) free(argv[i]);
	free(argv);
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// Format is the type an observed property is delivered as.
type Format int

const (
	FormatNone   Format = C.MPV_FORMAT_NONE
	FormatString Format = C.MPV_FORMAT_STRING
	FormatFlag   Format = C.MPV_FORMAT_FLAG
	FormatInt64  Format = C.MPV_FORMAT_INT64
	FormatDouble Format = C.MPV_FORMAT_DOUBLE
)

// EventID identifies an mpv event.
type EventID int

const (
	EventNone            EventID = C.MPV_EVENT_NONE
	EventShutdown        EventID = C.MPV_EVENT_SHUTDOWN
	EventLogMessage      EventID = C.MPV_EVENT_LOG_MESSAGE
	EventCommandReply    EventID = C.MPV_EVENT_COMMAND_REPLY
	EventStartFile       EventID = C.MPV_EVENT_START_FILE
	EventEndFile         EventID = C.MPV_EVENT_END_FILE
	EventFileLoaded      EventID = C.MPV_EVENT_FILE_LOADED
	EventIdle            EventID = C.MPV_EVENT_IDLE
	EventVideoReconfig   EventID = C.MPV_EVENT_VIDEO_RECONFIG
	EventAudioReconfig   EventID = C.MPV_EVENT_AUDIO_RECONFIG
	EventSeek            EventID = C.MPV_EVENT_SEEK
	EventPlaybackRestart EventID = C.MPV_EVENT_PLAYBACK_RESTART
	EventPropertyChange  EventID = C.MPV_EVENT_PROPERTY_CHANGE
)

// End-of-file reasons (EventEndFile).
const (
	EndEOF      = C.MPV_END_FILE_REASON_EOF
	EndStop     = C.MPV_END_FILE_REASON_STOP
	EndQuit     = C.MPV_END_FILE_REASON_QUIT
	EndError    = C.MPV_END_FILE_REASON_ERROR
	EndRedirect = C.MPV_END_FILE_REASON_REDIRECT
)

// Event is one mpv event, copied out of mpv's memory.
type Event struct {
	ID    EventID
	Error error
	Reply uint64

	// Property changes: the property and its new value, in Format.
	// Format is FormatNone when the property is unavailable, or when it
	// was observed without a format (only the change is reported).
	Name   string
	Format Format
	Double float64
	Int    int64
	Flag   bool
	String string

	// Log messages.
	LogPrefix, LogLevel, LogText string

	// End of file: the reason, and the playback error if any.
	EndReason int
}

// Handle is an mpv core.
type Handle struct {
	h      *C.mpv_handle
	wakeup cgo.Handle
}

// Create allocates an mpv core. Configure it with SetOption, then call
// Initialize; release it with Destroy.
func Create() (*Handle, error) {
	h := C.mpv_create()
	if h == nil {
		return nil, errors.New("mpv: mpv_create failed")
	}
	return &Handle{h: h}, nil
}

// SetOption sets an option before Initialize.
func (m *Handle) SetOption(name, value string) error {
	cname, cvalue := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(cname))
	defer C.free(unsafe.Pointer(cvalue))
	return check(C.mpv_set_option_string(m.h, cname, cvalue))
}

// Initialize starts the core.
func (m *Handle) Initialize() error {
	return check(C.mpv_initialize(m.h))
}

// RequestLog enables log messages at level ("no", "fatal" ... "trace").
func (m *Handle) RequestLog(level string) error {
	c := C.CString(level)
	defer C.free(unsafe.Pointer(c))
	return check(C.mpv_request_log_messages(m.h, c))
}

// SetWakeupCallback makes mpv call fn, from one of its own threads,
// whenever events are waiting. fn must not call back into mpv; it should
// only wake the thread that reads events.
func (m *Handle) SetWakeupCallback(fn func()) {
	if m.wakeup != 0 {
		m.wakeup.Delete()
	}
	m.wakeup = cgo.NewHandle(fn)
	C.goplay_set_wakeup(m.h, C.uintptr_t(m.wakeup))
}

//export goplayMpvWakeup
func goplayMpvWakeup(id C.uintptr_t) {
	if fn, ok := cgo.Handle(id).Value().(func()); ok {
		fn()
	}
}

// Command runs a command synchronously, e.g. Command("seek", "10").
func (m *Handle) Command(args ...string) error {
	argv := cArgv(args)
	defer C.goplay_argv_free(argv, C.int(len(args)))
	return check(C.goplay_command(m.h, argv))
}

// CommandAsync runs a command without waiting; its result arrives as an
// EventCommandReply carrying reply.
func (m *Handle) CommandAsync(reply uint64, args ...string) error {
	argv := cArgv(args)
	defer C.goplay_argv_free(argv, C.int(len(args)))
	return check(C.goplay_command_async(m.h, C.uint64_t(reply), argv))
}

func cArgv(args []string) **C.char {
	argv := C.goplay_argv(C.int(len(args)))
	for i, a := range args {
		C.goplay_argv_set(argv, C.int(i), C.CString(a))
	}
	return argv
}

// SetString sets a property from its string form.
func (m *Handle) SetString(name, value string) error {
	cname, cvalue := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(cname))
	defer C.free(unsafe.Pointer(cvalue))
	return check(C.mpv_set_property_string(m.h, cname, cvalue))
}

// SetDouble sets a numeric property.
func (m *Handle) SetDouble(name string, v float64) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cv := C.double(v)
	return check(C.mpv_set_property(m.h, cname, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&cv)))
}

// SetFlag sets a yes/no property.
func (m *Handle) SetFlag(name string, v bool) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cv := C.int(0)
	if v {
		cv = 1
	}
	return check(C.mpv_set_property(m.h, cname, C.MPV_FORMAT_FLAG, unsafe.Pointer(&cv)))
}

// String reads a property as a string; ok is false if it is unavailable.
// Lists and maps read this way come back as JSON.
func (m *Handle) String(name string) (string, bool) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cv := C.mpv_get_property_string(m.h, cname)
	if cv == nil {
		return "", false
	}
	defer C.mpv_free(unsafe.Pointer(cv))
	return C.GoString(cv), true
}

// Double reads a numeric property; ok is false if it is unavailable.
func (m *Handle) Double(name string) (float64, bool) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cv C.double
	if C.mpv_get_property(m.h, cname, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&cv)) < 0 {
		return 0, false
	}
	return float64(cv), true
}

// Int reads an integer property; ok is false if it is unavailable.
func (m *Handle) Int(name string) (int64, bool) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cv C.int64_t
	if C.mpv_get_property(m.h, cname, C.MPV_FORMAT_INT64, unsafe.Pointer(&cv)) < 0 {
		return 0, false
	}
	return int64(cv), true
}

// Flag reads a yes/no property; ok is false if it is unavailable.
func (m *Handle) Flag(name string) (bool, bool) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cv C.int
	if C.mpv_get_property(m.h, cname, C.MPV_FORMAT_FLAG, unsafe.Pointer(&cv)) < 0 {
		return false, false
	}
	return cv != 0, true
}

// JSON decodes a list or map property (track-list, chapter-list, ...)
// into v.
func (m *Handle) JSON(name string, v any) error {
	s, ok := m.String(name)
	if !ok {
		return fmt.Errorf("mpv: property %s unavailable", name)
	}
	return json.Unmarshal([]byte(s), v)
}

// Observe asks for EventPropertyChange events for name, delivered in
// format (FormatNone reports only that it changed). id is returned in
// the event's Reply.
func (m *Handle) Observe(id uint64, name string, format Format) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return check(C.mpv_observe_property(m.h, C.uint64_t(id), cname, C.mpv_format(format)))
}

// Wait returns the next event, waiting up to timeout seconds (0 polls).
// It must only be called from one goroutine at a time.
func (m *Handle) Wait(timeout float64) Event {
	ce := C.goplay_wait(m.h, C.double(timeout))
	ev := Event{
		ID:    EventID(ce.id),
		Reply: uint64(ce.reply),
	}
	if ce.error < 0 {
		ev.Error = errorFor(ce.error)
	}
	switch ev.ID {
	case EventPropertyChange:
		ev.Name = C.GoString(ce.name)
		ev.Format = Format(ce.format)
		switch ev.Format {
		case FormatDouble:
			ev.Double = float64(ce.dbl)
		case FormatInt64:
			ev.Int = int64(ce.i64)
		case FormatFlag:
			ev.Flag = ce.flag != 0
		case FormatString:
			ev.String = C.GoString(ce.str)
		}
	case EventLogMessage:
		ev.LogPrefix = C.GoString(ce.log_prefix)
		ev.LogLevel = C.GoString(ce.log_level)
		ev.LogText = C.GoString(ce.log_text)
	case EventEndFile:
		ev.EndReason = int(ce.end_reason)
		if ce.end_error < 0 {
			ev.Error = errorFor(ce.end_error)
		}
	}
	return ev
}

// Destroy shuts the core down and frees it. Free any render context
// first.
func (m *Handle) Destroy() {
	if m.h != nil {
		C.mpv_terminate_destroy(m.h)
		m.h = nil
	}
	if m.wakeup != 0 {
		m.wakeup.Delete()
		m.wakeup = 0
	}
}

// Error is an mpv error code.
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string { return "mpv: " + e.Msg }

func errorFor(code C.int) error {
	return &Error{Code: int(code), Msg: C.GoString(C.mpv_error_string(code))}
}

func check(code C.int) error {
	if code < 0 {
		return errorFor(code)
	}
	return nil
}
