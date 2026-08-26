package libmpv

/*
#include <stdlib.h>
#include <mpv/render.h>
#include <mpv/render_gl.h>

// GLFW is statically linked by go-gl/glfw; resolve the OpenGL proc-address
// getter against it rather than linking GLFW twice. glfwGetProcAddress
// dispatches to eglGetProcAddress when the current context is an EGL
// (ANGLE) context, so this also works for the OpenGL ES backend.
extern void* glfwGetProcAddress(const char* name);

static void* goplay_get_proc_address(void* ctx, const char* name) {
	(void)ctx;
	return glfwGetProcAddress(name);
}

static mpv_opengl_init_params goplay_opengl_init_params(void) {
	mpv_opengl_init_params p;
	p.get_proc_address = goplay_get_proc_address;
	p.get_proc_address_ctx = 0;
	return p;
}

// These helpers build the render param array on the C stack so no Go pointers
// cross the cgo boundary (mpv_render_param.data may point at Go-unfriendly
// structs).
static int goplay_render_context_create(mpv_handle* mpv, mpv_render_context** out) {
	const char* api_type = "opengl";
	mpv_opengl_init_params init = goplay_opengl_init_params();
	mpv_render_param params[] = {
		{MPV_RENDER_PARAM_API_TYPE, (void*)api_type},
		{MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &init},
		{MPV_RENDER_PARAM_INVALID, NULL},
	};
	return mpv_render_context_create(out, mpv, params);
}

static int goplay_render_context_render(mpv_render_context* rc, int w, int h) {
	mpv_opengl_fbo fbo = {0, w, h, 0};
	// The OpenGL default framebuffer has a bottom-left origin, so mpv must
	// flip the image vertically for it to appear the right way up.
	int flip_y = 1;
	mpv_render_param params[] = {
		{MPV_RENDER_PARAM_OPENGL_FBO, &fbo},
		{MPV_RENDER_PARAM_FLIP_Y, &flip_y},
		{MPV_RENDER_PARAM_INVALID, NULL},
	};
	// Pull pending renderer state (new frames, size changes) before drawing;
	// the render API requires this to make progress.
	mpv_render_context_update(rc);
	return mpv_render_context_render(rc, params);
}
*/
import "C"

// RenderContext wraps mpv_render_context. The associated GL context must be
// current on the calling thread whenever its methods are called.
type RenderContext struct {
	rc *C.mpv_render_context
}

// NewRenderContext creates the OpenGL render context for m. Call it only once
// the GL context is current.
func (m *Handle) NewRenderContext() (*RenderContext, error) {
	var rc *C.mpv_render_context
	if e := C.goplay_render_context_create(m.h, &rc); e < 0 {
		return nil, errorCode(e)
	}
	return &RenderContext{rc: rc}, nil
}

// Render draws the current video frame into the default framebuffer (fbo 0).
// fbw/fbh are the framebuffer pixel dimensions.
func (r *RenderContext) Render(fbw, fbh int) error {
	if e := C.goplay_render_context_render(r.rc, C.int(fbw), C.int(fbh)); e < 0 {
		return errorCode(e)
	}
	return nil
}

// Free releases the render context. Must happen before the mpv handle is
// destroyed.
func (r *RenderContext) Free() {
	if r.rc != nil {
		C.mpv_render_context_free(r.rc)
		r.rc = nil
	}
}
