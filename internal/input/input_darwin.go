//go:build darwin

package input

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices -framework AppKit

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#import <AppKit/AppKit.h>
#include <unistd.h>

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
    usleep(30000); // 30ms 대기

    // Click down
    CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, down);
    CFRelease(down);
    usleep(50000); // 50ms 대기

    // Click up
    CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(up);
    usleep(30000); // 30ms 대기
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
    usleep(50000); // 50ms 대기 (키 다운 후)
    CGEventPost(kCGHIDEventTap, up);
    usleep(50000); // 50ms 대기 (키 업 후)

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

const char* getClipboard() {
    @autoreleasepool {
        NSPasteboard* pb = [NSPasteboard generalPasteboard];
        NSString* str = [pb stringForType:NSPasteboardTypeString];
        if (str == nil) {
            return "";
        }
        return strdup([str UTF8String]); // caller must free
    }
}

// AppleScript로 Return 키 입력 (CGEvent보다 안정적)
void pressReturnKey() {
    @autoreleasepool {
        NSString *script = @"tell application \"System Events\" to keystroke return";
        NSAppleScript *appleScript = [[NSAppleScript alloc] initWithSource:script];
        [appleScript executeAndReturnError:nil];
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
	keyCodeA       = 0  // A 키
	keyCodeC       = 8  // C 키
	keyCodeDelete  = 51 // Backspace/Delete
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
	time.Sleep(300 * time.Millisecond) // 클립보드 안전 대기 (파이썬: 0.3초)

	// Cmd+V (붙여넣기)
	C.pressKey(C.int(keyCodeV), C.int(flagCommand))
}

func pressEnter() {
	C.pressReturnKey() // AppleScript 방식 사용
}

func clearInput() {
	// Cmd+A (전체 선택)
	C.pressKey(C.int(keyCodeA), C.int(flagCommand))
	time.Sleep(50 * time.Millisecond)
	// Delete (삭제)
	C.pressKey(C.int(keyCodeDelete), 0)
}

func selectAll() {
	// Cmd+A (전체 선택)
	C.pressKey(C.int(keyCodeA), C.int(flagCommand))
}

func copySelection() {
	// Cmd+C (복사)
	C.pressKey(C.int(keyCodeC), C.int(flagCommand))
}

func getClipboard() string {
	cStr := C.getClipboard()
	if cStr == nil {
		return ""
	}
	str := C.GoString(cStr)
	C.free(unsafe.Pointer(cStr))
	return str
}
