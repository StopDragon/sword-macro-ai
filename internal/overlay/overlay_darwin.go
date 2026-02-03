//go:build darwin

package overlay

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

static NSWindow *ocrWindow = nil;
static NSWindow *inputWindow = nil;
static NSWindow *statusWindow = nil;
static NSTextField *statusLabel = nil;
static BOOL appInitialized = NO;

// Run loop pump - CLI 앱에서 Cocoa 이벤트 처리
void PumpRunLoop() {
    @autoreleasepool {
        NSDate *future = [NSDate dateWithTimeIntervalSinceNow:0.1];
        [[NSRunLoop currentRunLoop] runUntilDate:future];
    }
}

// OCR 영역 오버레이 (빨간색)
void ShowOCRRegion(int x, int y, int width, int height) {
    @autoreleasepool {
        if (ocrWindow != nil) {
            [ocrWindow close];
            ocrWindow = nil;
        }

        NSRect frame = NSMakeRect(x, [[NSScreen mainScreen] frame].size.height - y - height, width, height);

        ocrWindow = [[NSWindow alloc]
            initWithContentRect:frame
            styleMask:NSWindowStyleMaskBorderless
            backing:NSBackingStoreBuffered
            defer:NO];

        [ocrWindow setLevel:NSScreenSaverWindowLevel];
        [ocrWindow setBackgroundColor:[NSColor clearColor]];
        [ocrWindow setOpaque:NO];
        [ocrWindow setIgnoresMouseEvents:YES];
        [ocrWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary];

        NSView *contentView = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, height)];
        contentView.wantsLayer = YES;
        contentView.layer.borderColor = [[NSColor redColor] CGColor];
        contentView.layer.borderWidth = 3.0;
        contentView.layer.backgroundColor = [[NSColor colorWithRed:1.0 green:0.0 blue:0.0 alpha:0.1] CGColor];

        [ocrWindow setContentView:contentView];
        [ocrWindow orderFrontRegardless];
    }
}

// 입력창 영역 오버레이 (초록색)
void ShowInputRegion(int x, int y, int width, int height) {
    @autoreleasepool {
        if (inputWindow != nil) {
            [inputWindow close];
            inputWindow = nil;
        }

        NSRect frame = NSMakeRect(x, [[NSScreen mainScreen] frame].size.height - y - height, width, height);

        inputWindow = [[NSWindow alloc]
            initWithContentRect:frame
            styleMask:NSWindowStyleMaskBorderless
            backing:NSBackingStoreBuffered
            defer:NO];

        [inputWindow setLevel:NSScreenSaverWindowLevel];
        [inputWindow setBackgroundColor:[NSColor clearColor]];
        [inputWindow setOpaque:NO];
        [inputWindow setIgnoresMouseEvents:YES];
        [inputWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary];

        NSView *contentView = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, height)];
        contentView.wantsLayer = YES;
        contentView.layer.borderColor = [[NSColor greenColor] CGColor];
        contentView.layer.borderWidth = 3.0;
        contentView.layer.backgroundColor = [[NSColor colorWithRed:0.0 green:1.0 blue:0.0 alpha:0.1] CGColor];

        [inputWindow setContentView:contentView];
        [inputWindow orderFrontRegardless];
    }
}

// 상태 패널 (우측 하단)
void ShowStatusPanel(int x, int y, int width, int height) {
    @autoreleasepool {
        if (statusWindow != nil) {
            [statusWindow close];
            statusWindow = nil;
            statusLabel = nil;
        }

        NSRect frame = NSMakeRect(x, [[NSScreen mainScreen] frame].size.height - y - height, width, height);

        statusWindow = [[NSWindow alloc]
            initWithContentRect:frame
            styleMask:NSWindowStyleMaskBorderless
            backing:NSBackingStoreBuffered
            defer:NO];

        [statusWindow setLevel:NSScreenSaverWindowLevel];
        [statusWindow setBackgroundColor:[NSColor colorWithRed:0.1 green:0.1 blue:0.1 alpha:0.9]];
        [statusWindow setOpaque:NO];
        [statusWindow setIgnoresMouseEvents:YES];
        [statusWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary];

        statusLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(10, 10, width - 20, height - 20)];
        [statusLabel setBezeled:NO];
        [statusLabel setDrawsBackground:NO];
        [statusLabel setEditable:NO];
        [statusLabel setSelectable:NO];
        [statusLabel setTextColor:[NSColor whiteColor]];
        [statusLabel setFont:[NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightMedium]];
        [statusLabel setStringValue:@"🎮 대기 중..."];

        [[statusWindow contentView] addSubview:statusLabel];
        [statusWindow orderFrontRegardless];
    }
}

// 상태 텍스트 업데이트
void UpdateStatus(const char *text) {
    @autoreleasepool {
        if (statusLabel != nil) {
            NSString *nsText = [NSString stringWithUTF8String:text];
            [statusLabel setStringValue:nsText];
            [statusWindow display];
        }
    }
}

// 모든 오버레이 숨기기
void HideAllOverlays() {
    @autoreleasepool {
        if (ocrWindow != nil) {
            [ocrWindow close];
            ocrWindow = nil;
        }
        if (inputWindow != nil) {
            [inputWindow close];
            inputWindow = nil;
        }
        if (statusWindow != nil) {
            [statusWindow close];
            statusWindow = nil;
            statusLabel = nil;
        }
    }
}

void InitApp() {
    @autoreleasepool {
        if (!appInitialized) {
            [NSApplication sharedApplication];
            [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
            appInitialized = YES;
        }
    }
}
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"
)

var initialized = false

// Init 오버레이 시스템 초기화
func Init() {
	if !initialized {
		C.InitApp()
		C.PumpRunLoop()
		time.Sleep(50 * time.Millisecond)
		initialized = true
	}
}

// pumpEvents Cocoa 이벤트 루프 처리
func pumpEvents() {
	C.PumpRunLoop()
}

// Show OCR 캡처 영역 오버레이 표시 (하위 호환)
func Show(x, y, width, height int) {
	ShowOCRRegion(x, y, width, height)
}

// Hide 오버레이 숨기기 (하위 호환)
func Hide() {
	HideAll()
}

// ShowOCRRegion OCR 영역 표시 (빨간색)
func ShowOCRRegion(x, y, width, height int) {
	if !initialized {
		Init()
	}
	C.ShowOCRRegion(C.int(x), C.int(y), C.int(width), C.int(height))
	pumpEvents()
}

// ShowInputRegion 입력창 영역 표시 (초록색)
func ShowInputRegion(x, y, width, height int) {
	if !initialized {
		Init()
	}
	C.ShowInputRegion(C.int(x), C.int(y), C.int(width), C.int(height))
	pumpEvents()
}

// ShowStatusPanel 상태 패널 표시
func ShowStatusPanel(x, y, width, height int) {
	if !initialized {
		Init()
	}
	C.ShowStatusPanel(C.int(x), C.int(y), C.int(width), C.int(height))
	pumpEvents()
}

// ShowAll 모든 오버레이 표시 (OCR 영역, 입력창 영역, 상태 패널)
func ShowAll(ocrX, ocrY, ocrW, ocrH, inputX, inputY, inputW, inputH int) {
	if !initialized {
		Init()
	}

	// OCR 영역 (빨간색)
	C.ShowOCRRegion(C.int(ocrX), C.int(ocrY), C.int(ocrW), C.int(ocrH))

	// 입력창 영역 (초록색)
	C.ShowInputRegion(C.int(inputX), C.int(inputY), C.int(inputW), C.int(inputH))

	// 상태 패널 (OCR 영역 오른쪽)
	statusX := ocrX + ocrW + 10
	statusY := ocrY
	statusW := 280
	statusH := 150
	C.ShowStatusPanel(C.int(statusX), C.int(statusY), C.int(statusW), C.int(statusH))

	// 이벤트 처리
	pumpEvents()
	time.Sleep(100 * time.Millisecond)
	pumpEvents()
}

// UpdateStatus 상태 텍스트 업데이트
func UpdateStatus(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	cText := C.CString(text)
	C.UpdateStatus(cText)
	C.free(unsafe.Pointer(cText))
	pumpEvents()
}

// HideAll 모든 오버레이 숨기기
func HideAll() {
	C.HideAllOverlays()
	pumpEvents()
}

// ShowForDuration 지정 시간 동안 오버레이 표시
func ShowForDuration(x, y, width, height int, duration time.Duration) {
	Show(x, y, width, height)
	time.Sleep(duration)
	Hide()
}
