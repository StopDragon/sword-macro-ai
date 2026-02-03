// +build darwin

package overlay

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore

#import <Cocoa/Cocoa.h>

static NSWindow *overlayWindow = nil;

void ShowOverlay(int x, int y, int width, int height) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (overlayWindow != nil) {
            [overlayWindow close];
            overlayWindow = nil;
        }

        NSRect frame = NSMakeRect(x, [[NSScreen mainScreen] frame].size.height - y - height, width, height);

        overlayWindow = [[NSWindow alloc]
            initWithContentRect:frame
            styleMask:NSWindowStyleMaskBorderless
            backing:NSBackingStoreBuffered
            defer:NO];

        [overlayWindow setLevel:NSFloatingWindowLevel];
        [overlayWindow setBackgroundColor:[NSColor clearColor]];
        [overlayWindow setOpaque:NO];
        [overlayWindow setIgnoresMouseEvents:YES];
        [overlayWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces];

        // 빨간색 테두리 뷰 생성
        NSView *contentView = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, height)];
        contentView.wantsLayer = YES;
        contentView.layer.borderColor = [[NSColor redColor] CGColor];
        contentView.layer.borderWidth = 3.0;
        contentView.layer.backgroundColor = [[NSColor colorWithRed:1.0 green:0.0 blue:0.0 alpha:0.1] CGColor];

        [overlayWindow setContentView:contentView];
        [overlayWindow makeKeyAndOrderFront:nil];
    });
}

void HideOverlay() {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (overlayWindow != nil) {
            [overlayWindow close];
            overlayWindow = nil;
        }
    });
}

void InitApp() {
    // NSApplication 초기화 (메인 스레드에서)
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    });
}
*/
import "C"
import (
	"time"
)

var initialized = false

// Init 오버레이 시스템 초기화
func Init() {
	if !initialized {
		C.InitApp()
		time.Sleep(100 * time.Millisecond)
		initialized = true
	}
}

// Show OCR 캡처 영역 오버레이 표시
func Show(x, y, width, height int) {
	if !initialized {
		Init()
	}
	C.ShowOverlay(C.int(x), C.int(y), C.int(width), C.int(height))
}

// Hide 오버레이 숨기기
func Hide() {
	C.HideOverlay()
}

// ShowForDuration 지정 시간 동안 오버레이 표시
func ShowForDuration(x, y, width, height int, duration time.Duration) {
	Show(x, y, width, height)
	time.Sleep(duration)
	Hide()
}
