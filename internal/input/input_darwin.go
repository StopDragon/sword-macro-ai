//go:build darwin

package input

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework AppKit

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#import <AppKit/AppKit.h>

void moveMouse(int x, int y) {
    CGPoint point = CGPointMake(x, y);
    CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, point, kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, move);
    CFRelease(move);
}

void clickMouse(int x, int y) {
    CGPoint point = CGPointMake(x, y);

    // Move first
    CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, point, kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, move);
    CFRelease(move);

    // Click down
    CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, down);
    CFRelease(down);

    // Click up
    CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(up);
}

void getMousePosition(int* x, int* y) {
    CGEventRef event = CGEventCreate(NULL);
    CGPoint point = CGEventGetLocation(event);
    *x = (int)point.x;
    *y = (int)point.y;
    CFRelease(event);
}

void pressKey(int keyCode, int flags) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, true);
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, false);

    if (flags != 0) {
        CGEventSetFlags(down, (CGEventFlags)flags);
        CGEventSetFlags(up, (CGEventFlags)flags);
    }

    CGEventPost(kCGHIDEventTap, down);
    CGEventPost(kCGHIDEventTap, up);

    CFRelease(down);
    CFRelease(up);
}

void setClipboard(const char* text) {
    @autoreleasepool {
        NSPasteboard* pb = [NSPasteboard generalPasteboard];
        [pb clearContents];
        [pb setString:[NSString stringWithUTF8String:text] forType:NSPasteboardTypeString];
    }
}

// Key codes
// Enter: 36
// V: 9
// Command modifier: kCGEventFlagMaskCommand = 0x100000
*/
import "C"

import (
	"time"
	"unsafe"
)

const (
	keyCodeEnter   = 36
	keyCodeV       = 9
	flagCommand    = 0x100000 // kCGEventFlagMaskCommand
)

func move(x, y int) {
	C.moveMouse(C.int(x), C.int(y))
}

func click(x, y int) {
	C.clickMouse(C.int(x), C.int(y))
}

func getMousePos() (int, int) {
	var x, y C.int
	C.getMousePosition(&x, &y)
	return int(x), int(y)
}

func typeText(text string) {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	// 클립보드에 복사
	C.setClipboard(cText)
	time.Sleep(50 * time.Millisecond)

	// Cmd+V (붙여넣기)
	C.pressKey(C.int(keyCodeV), C.int(flagCommand))
}

func pressEnter() {
	C.pressKey(C.int(keyCodeEnter), 0)
}
