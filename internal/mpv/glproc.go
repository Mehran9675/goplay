package mpv

/*
#cgo linux LDFLAGS: -ldl
#ifndef _WIN32
#define _GNU_SOURCE
#endif
#include <stdlib.h>

#ifdef _WIN32
#include <windows.h>

// ANGLE: the window's libEGL.dll resolves EGL and every GLES function;
// libGLESv2.dll's exports are the fallback.
static void* goplay_gl_proc(const char* name) {
	typedef void* (WINAPI *getproc_t)(const char*);
	static getproc_t egl;
	static HMODULE gles;
	if (egl == NULL) {
		HMODULE m = GetModuleHandleA("libEGL.dll");
		if (m != NULL) {
			egl = (getproc_t)GetProcAddress(m, "eglGetProcAddress");
		}
		gles = GetModuleHandleA("libGLESv2.dll");
	}
	void* p = egl != NULL ? egl(name) : NULL;
	if (p == NULL && gles != NULL) {
		p = (void*)GetProcAddress(gles, name);
	}
	return p;
}
#else
#include <dlfcn.h>

// Linux: EGL, which Gio's window already loaded. macOS: the OpenGL
// framework's symbols, found in the process.
static void* goplay_gl_proc(const char* name) {
#ifdef __linux__
	typedef void* (*getproc_t)(const char*);
	static getproc_t egl;
	if (egl == NULL) {
		egl = (getproc_t)dlsym(RTLD_DEFAULT, "eglGetProcAddress");
	}
	if (egl != NULL) {
		void* p = egl(name);
		if (p != NULL) {
			return p;
		}
	}
#endif
	return dlsym(RTLD_DEFAULT, name);
}
#endif
*/
import "C"

import "unsafe"

// GLProcAddress resolves an OpenGL (ES) function of the window's
// context: through ANGLE's libEGL on Windows, EGL on Linux and the
// OpenGL framework on macOS. It suits NewRenderContext when the context
// is the one Gio made.
func GLProcAddress(name string) unsafe.Pointer {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	return C.goplay_gl_proc(cs)
}
