//go:build darwin

#import <Cocoa/Cocoa.h>
#import <IOKit/pwr_mgt/IOPMLib.h>
#import <objc/runtime.h>

extern void goplayOpenedFile(char* path);

// goplay_style_window gives GLFW's borderless window a transparent,
// full-size title bar: the window keeps native rounded corners, shadow,
// edge resizing and traffic lights, and goplay's content fills it all.
void goplay_style_window(void* ptr) {
	NSWindow* window = (__bridge NSWindow*)ptr;
	window.styleMask |= NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
		NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable |
		NSWindowStyleMaskFullSizeContentView;
	window.titlebarAppearsTransparent = YES;
	window.titleVisibility = NSWindowTitleHidden;
	window.movableByWindowBackground = NO;
	window.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
	window.collectionBehavior |= NSWindowCollectionBehaviorFullScreenPrimary;
	window.backgroundColor = [NSColor blackColor];
}

void goplay_set_traffic_lights(void* ptr, int visible) {
	NSWindow* window = (__bridge NSWindow*)ptr;
	for (NSNumber* b in @[@(NSWindowCloseButton), @(NSWindowMiniaturizeButton), @(NSWindowZoomButton)]) {
		[[window standardWindowButton:(NSWindowButton)b.integerValue] setHidden:!visible];
	}
}

void goplay_toggle_fullscreen(void* ptr) {
	NSWindow* window = (__bridge NSWindow*)ptr;
	[window toggleFullScreen:nil];
}

int goplay_is_fullscreen(void* ptr) {
	NSWindow* window = (__bridge NSWindow*)ptr;
	return (window.styleMask & NSWindowStyleMaskFullScreen) != 0;
}

// Files opened from Finder arrive as Apple Events at the application
// delegate, which GLFW owns and doesn't forward. goplay adds the two
// delegate methods macOS calls for them to GLFW's delegate class.

static void goplay_open_urls(id self, SEL _cmd, NSApplication* app, NSArray<NSURL*>* urls) {
	for (NSURL* url in urls) {
		if (url.isFileURL) {
			goplayOpenedFile((char*)url.path.fileSystemRepresentation);
		} else {
			goplayOpenedFile((char*)url.absoluteString.UTF8String);
		}
	}
}

static BOOL goplay_open_file(id self, SEL _cmd, NSApplication* app, NSString* path) {
	goplayOpenedFile((char*)path.fileSystemRepresentation);
	return YES;
}

void goplay_install_open_handler(void) {
	id delegate = NSApp.delegate;
	if (delegate == nil) {
		return;
	}
	Class cls = [delegate class];
	class_addMethod(cls, @selector(application:openURLs:), (IMP)goplay_open_urls, "v@:@@");
	class_addMethod(cls, @selector(application:openFile:), (IMP)goplay_open_file, "c@:@@");
}

// Keeping the display awake during playback.

static IOPMAssertionID goplay_assertion = kIOPMNullAssertionID;

void goplay_keep_awake(int on) {
	if (on && goplay_assertion == kIOPMNullAssertionID) {
		IOPMAssertionCreateWithName(kIOPMAssertionTypePreventUserIdleDisplaySleep,
			kIOPMAssertionLevelOn, CFSTR("Playing video"), &goplay_assertion);
	} else if (!on && goplay_assertion != kIOPMNullAssertionID) {
		IOPMAssertionRelease(goplay_assertion);
		goplay_assertion = kIOPMNullAssertionID;
	}
}
