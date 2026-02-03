// +build darwin

package overlay

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore

#import <Cocoa/Cocoa.h>

static NSWindow *ocrWindow = nil;
static NSWindow *inputWindow = nil;
static NSWindow *statusWindow = nil;
static NSTextField *statusLabel = nil;

// OCR 영역 오버레이 (빨간색)
void ShowOCRRegion(int x, int y, int width, int height) {
    dispatch_async(dispatch_get_main_queue(), ^{
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

        [ocrWindow setLevel:NSFloatingWindowLevel];
        [ocrWindow setBackgroundColor:[NSColor clearColor]];
        [ocrWindow setOpaque:NO];
        [ocrWindow setIgnoresMouseEvents:YES];
        [ocrWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces];

        NSView *contentView = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, height)];
        contentView.wantsLayer = YES;
        contentView.layer.borderColor = [[NSColor redColor] CGColor];
        contentView.layer.borderWidth = 2.0;
        contentView.layer.backgroundColor = [[NSColor colorWithRed:1.0 green:0.0 blue:0.0 alpha:0.05] CGColor];

        [ocrWindow setContentView:contentView];
        [ocrWindow makeKeyAndOrderFront:nil];
    });
}

// 입력창 영역 오버레이 (초록색)
void ShowInputRegion(int x, int y, int width, int height) {
    dispatch_async(dispatch_get_main_queue(), ^{
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

        [inputWindow setLevel:NSFloatingWindowLevel];
        [inputWindow setBackgroundColor:[NSColor clearColor]];
        [inputWindow setOpaque:NO];
        [inputWindow setIgnoresMouseEvents:YES];
        [inputWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces];

        NSView *contentView = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, height)];
        contentView.wantsLayer = YES;
        contentView.layer.borderColor = [[NSColor greenColor] CGColor];
        contentView.layer.borderWidth = 2.0;
        contentView.layer.backgroundColor = [[NSColor colorWithRed:0.0 green:1.0 blue:0.0 alpha:0.05] CGColor];

        [inputWindow setContentView:contentView];
        [inputWindow makeKeyAndOrderFront:nil];
    });
}

// 상태 패널 (우측 하단)
void ShowStatusPanel(int x, int y, int width, int height) {
    dispatch_async(dispatch_get_main_queue(), ^{
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

        [statusWindow setLevel:NSFloatingWindowLevel];
        [statusWindow setBackgroundColor:[NSColor colorWithRed:0.0 green:0.0 blue:0.0 alpha:0.8]];
        [statusWindow setOpaque:NO];
        [statusWindow setIgnoresMouseEvents:YES];
        [statusWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces];

        statusLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(10, 10, width - 20, height - 20)];
        [statusLabel setBezeled:NO];
        [statusLabel setDrawsBackground:NO];
        [statusLabel setEditable:NO];
        [statusLabel setSelectable:NO];
        [statusLabel setTextColor:[NSColor whiteColor]];
        [statusLabel setFont:[NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular]];
        [statusLabel setStringValue:@"🎮 대기 중..."];

        [[statusWindow contentView] addSubview:statusLabel];
        [statusWindow makeKeyAndOrderFront:nil];
    });
}

// 상태 텍스트 업데이트
void UpdateStatus(const char *text) {
    NSString *nsText = [NSString stringWithUTF8String:text];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusLabel != nil) {
            [statusLabel setStringValue:nsText];
        }
    });
}

// 모든 오버레이 숨기기
void HideAllOverlays() {
    dispatch_async(dispatch_get_main_queue(), ^{
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
    });
}

void InitApp() {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    });
}
*/
import "C"
import (
	"fmt"
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
}

// ShowInputRegion 입력창 영역 표시 (초록색)
func ShowInputRegion(x, y, width, height int) {
	if !initialized {
		Init()
	}
	C.ShowInputRegion(C.int(x), C.int(y), C.int(width), C.int(height))
}

// ShowStatusPanel 상태 패널 표시
func ShowStatusPanel(x, y, width, height int) {
	if !initialized {
		Init()
	}
	C.ShowStatusPanel(C.int(x), C.int(y), C.int(width), C.int(height))
}

// ShowAll 모든 오버레이 표시 (OCR 영역, 입력창 영역, 상태 패널)
func ShowAll(ocrX, ocrY, ocrW, ocrH, inputX, inputY, inputW, inputH int) {
	if !initialized {
		Init()
	}

	// OCR 영역 (빨간색)
	ShowOCRRegion(ocrX, ocrY, ocrW, ocrH)

	// 입력창 영역 (초록색)
	ShowInputRegion(inputX, inputY, inputW, inputH)

	// 상태 패널 (OCR 영역 오른쪽)
	statusX := ocrX + ocrW + 10
	statusY := ocrY
	statusW := 280
	statusH := 150
	ShowStatusPanel(statusX, statusY, statusW, statusH)
}

// UpdateStatus 상태 텍스트 업데이트
func UpdateStatus(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	C.UpdateStatus(C.CString(text))
}

// HideAll 모든 오버레이 숨기기
func HideAll() {
	C.HideAllOverlays()
}

// ShowForDuration 지정 시간 동안 오버레이 표시
func ShowForDuration(x, y, width, height int, duration time.Duration) {
	Show(x, y, width, height)
	time.Sleep(duration)
	Hide()
}
