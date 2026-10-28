package mpv

/*
#include <stdint.h>
#include <mpv/render.h>
#include <mpv/render_gl.h>

// OpenGL functions are resolved by a Go function the caller supplies
// (the windowing layer knows how), reached through goplayMpvGetProc.
extern void* goplayMpvGetProc(uintptr_t id, char* name);
extern void goplayMpvRenderUpdate(uintptr_t id);

static void* goplay_get_proc_address(void* ctx, const char* name) {
	return goplayMpvGetProc((uintptr_t)ctx, (char*)name);
}

static void goplay_update_cb(void* ctx) {
	goplayMpvRenderUpdate((uintptr_t)ctx);
}

static int goplay_render_create(mpv_handle* mpv, mpv_render_context** out, uintptr_t getproc) {
	const char* api = MPV_RENDER_API_TYPE_OPENGL;
	mpv_opengl_init_params init = {goplay_get_proc_address, (void*)getproc};
	mpv_render_param params[] = {
		{MPV_RENDER_PARAM_API_TYPE, (void*)api},
		{MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &init},
		{MPV_RENDER_PARAM_INVALID, 0},
	};
	return mpv_render_context_create(out, mpv, params);
}

static void goplay_render_set_update(mpv_render_context* rc, uintptr_t id) {
	mpv_render_context_set_update_callback(rc, goplay_update_cb, (void*)id);
}

static int goplay_render(mpv_render_context* rc, int fbo, int w, int h, int flip) {
	mpv_opengl_fbo target = {fbo, w, h, 0};
	mpv_render_param params[] = {
		{MPV_RENDER_PARAM_OPENGL_FBO, &target},
		{MPV_RENDER_PARAM_FLIP_Y, &flip},
		{MPV_RENDER_PARAM_INVALID, 0},
	};
	return mpv_render_context_render(rc, params);
}
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

// RenderContext draws video with OpenGL into the current GL context. Its
// methods must be called on the thread where that context is current.
type RenderContext struct {
	rc      *C.mpv_render_context
	update  cgo.Handle
	getProc cgo.Handle
}

// GetProcAddress resolves an OpenGL function by name, as the context's
// platform layer does (glfwGetProcAddress, eglGetProcAddress, ...).
type GetProcAddress func(name string) unsafe.Pointer

// NewRenderContext creates an OpenGL render context for m. The GL
// context must be current; getProc resolves its functions.
func (m *Handle) NewRenderContext(getProc GetProcAddress) (*RenderContext, error) {
	r := &RenderContext{getProc: cgo.NewHandle(getProc)}
	if err := check(C.goplay_render_create(m.h, &r.rc, C.uintptr_t(r.getProc))); err != nil {
		r.getProc.Delete()
		return nil, err
	}
	return r, nil
}

//export goplayMpvGetProc
func goplayMpvGetProc(id C.uintptr_t, name *C.char) unsafe.Pointer {
	fn, ok := cgo.Handle(id).Value().(GetProcAddress)
	if !ok {
		return nil
	}
	return fn(C.GoString(name))
}

// SetUpdateCallback makes mpv call fn, from one of its own threads,
// when a new frame should be drawn. fn must only wake the render thread,
// which then calls Update.
func (r *RenderContext) SetUpdateCallback(fn func()) {
	if r.update != 0 {
		r.update.Delete()
	}
	r.update = cgo.NewHandle(fn)
	C.goplay_render_set_update(r.rc, C.uintptr_t(r.update))
}

//export goplayMpvRenderUpdate
func goplayMpvRenderUpdate(id C.uintptr_t) {
	if fn, ok := cgo.Handle(id).Value().(func()); ok {
		fn()
	}
}

// Update processes pending render state and reports whether a new video
// frame is ready to be drawn. Call it after the update callback fires.
func (r *RenderContext) Update() bool {
	return C.mpv_render_context_update(r.rc)&C.MPV_RENDER_UPDATE_FRAME != 0
}

// Render draws the current frame into framebuffer fbo (0 for the
// window), w by h pixels. flip renders upside down, as the default
// framebuffer's bottom-left origin needs.
func (r *RenderContext) Render(fbo, w, h int, flip bool) error {
	f := C.int(0)
	if flip {
		f = 1
	}
	return check(C.goplay_render(r.rc, C.int(fbo), C.int(w), C.int(h), f))
}

// ReportSwap tells mpv a frame was presented, for its frame timing.
func (r *RenderContext) ReportSwap() {
	C.mpv_render_context_report_swap(r.rc)
}

// Free releases the render context. Do it before destroying the core.
func (r *RenderContext) Free() {
	if r.rc != nil {
		C.mpv_render_context_free(r.rc)
		r.rc = nil
	}
	if r.update != 0 {
		r.update.Delete()
		r.update = 0
	}
	if r.getProc != 0 {
		r.getProc.Delete()
		r.getProc = 0
	}
}
