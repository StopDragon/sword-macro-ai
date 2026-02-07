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
static NSWindow *controlWindow = nil;
static BOOL appInitialized = NO;

// 버튼 클릭 상태 (Go에서 폴링)
static volatile int pauseClicked = 0;
static volatile int stopClicked = 0;
static volatile int restartClicked = 0;

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
        contentView.layer.backgroundColor = [[NSColor clearColor] CGColor];

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
        contentView.layer.backgroundColor = [[NSColor clearColor] CGColor];

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
        // 멀티라인 지원
        [statusLabel setLineBreakMode:NSLineBreakByWordWrapping];
        [statusLabel setMaximumNumberOfLines:0];  // 0 = 무제한
        [[statusLabel cell] setWraps:YES];
        [[statusLabel cell] setScrollable:NO];
        [statusLabel setStringValue:@"🎮 대기 중..."];

        [[statusWindow contentView] addSubview:statusLabel];
        [statusWindow orderFrontRegardless];
    }
}

// 상태 텍스트 업데이트 (동기 방식 - CLI 앱에서 dispatch_async는 동작하지 않음)
void UpdateStatus(const char *text) {
    @autoreleasepool {
        if (statusLabel != nil && statusWindow != nil) {
            NSString *nsText = [NSString stringWithUTF8String:text];
            // CLI 앱에서는 dispatch_async가 제대로 동작하지 않으므로 직접 업데이트
            [statusLabel setStringValue:nsText];
            [statusLabel setNeedsDisplay:YES];
            [statusWindow display];
            [statusWindow orderFrontRegardless];
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
        if (controlWindow != nil) {
            [controlWindow close];
            controlWindow = nil;
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

// 버튼 클릭 핸들러 클래스
@interface ButtonHandler : NSObject
- (void)pauseClicked:(id)sender;
- (void)stopClicked:(id)sender;
- (void)restartClicked:(id)sender;
@end

@implementation ButtonHandler
- (void)pauseClicked:(id)sender {
    pauseClicked = 1;
}
- (void)stopClicked:(id)sender {
    stopClicked = 1;
}
- (void)restartClicked:(id)sender {
    restartClicked = 1;
}
@end

static ButtonHandler *buttonHandler = nil;

// 단축키 안내 패널 표시 (초록 테두리 아래)
void ShowInfoPanel(int x, int y, const char *text) {
    @autoreleasepool {
        if (controlWindow != nil) {
            [controlWindow close];
            controlWindow = nil;
        }

        int width = 200;
        int height = 25;

        NSRect frame = NSMakeRect(x, [[NSScreen mainScreen] frame].size.height - y - height, width, height);

        controlWindow = [[NSWindow alloc]
            initWithContentRect:frame
            styleMask:NSWindowStyleMaskBorderless
            backing:NSBackingStoreBuffered
            defer:NO];

        [controlWindow setLevel:NSScreenSaverWindowLevel];
        [controlWindow setBackgroundColor:[NSColor colorWithRed:0.15 green:0.15 blue:0.15 alpha:0.85]];
        [controlWindow setOpaque:NO];
        [controlWindow setIgnoresMouseEvents:YES];
        [controlWindow setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary];

        NSTextField *label = [[NSTextField alloc] initWithFrame:NSMakeRect(8, 2, width - 16, height - 4)];
        [label setBezeled:NO];
        [label setDrawsBackground:NO];
        [label setEditable:NO];
        [label setSelectable:NO];
        [label setTextColor:[NSColor colorWithRed:0.8 green:0.8 blue:0.8 alpha:1.0]];
        [label setFont:[NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightMedium]];
        [label setStringValue:[NSString stringWithUTF8String:text]];

        [[controlWindow contentView] addSubview:label];
        [controlWindow orderFrontRegardless];
    }
}

// 버튼 클릭 상태 확인
int CheckPauseClicked() {
    if (pauseClicked) {
        pauseClicked = 0;
        return 1;
    }
    return 0;
}

int CheckStopClicked() {
    if (stopClicked) {
        stopClicked = 0;
        return 1;
    }
    return 0;
}

int CheckRestartClicked() {
    if (restartClicked) {
        restartClicked = 0;
        return 1;
    }
    return 0;
}

// 컨트롤 패널 숨기기
void HideControlPanel() {
    @autoreleasepool {
        if (controlWindow != nil) {
            [controlWindow close];
            controlWindow = nil;
        }
    }
}
*/
import "C"
import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var initialized = false

// 로그 버퍼 (CLI 터미널 스타일)
var (
	logBuffer    []string
	logMutex     sync.Mutex
	maxLogLines  = 25 // 상태 패널에 표시할 최대 라인 수
)

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

	// 상태 패널 (OCR 영역 오른쪽, OCR과 같은 높이)
	statusX := ocrX + ocrW + 10
	statusY := ocrY
	statusW := 280
	statusH := ocrH // OCR 영역과 동일한 높이
	C.ShowStatusPanel(C.int(statusX), C.int(statusY), C.int(statusW), C.int(statusH))

	// 이벤트 처리
	pumpEvents()
	time.Sleep(100 * time.Millisecond)
	pumpEvents()
}

// UpdateStatus 상태 텍스트 업데이트 (로그 스타일 - 아래에서 위로 쌓임)
func UpdateStatus(format string, args ...interface{}) {
	if !initialized {
		return // 초기화되지 않았으면 무시
	}
	text := fmt.Sprintf(format, args...)

	logMutex.Lock()
	// 새 텍스트의 각 라인을 버퍼에 추가
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if line != "" { // 빈 라인 제외
			logBuffer = append(logBuffer, line)
		}
	}
	// 빈 라인 하나 추가 (메시지 구분)
	logBuffer = append(logBuffer, "")

	// 최대 라인 수 유지 (오래된 것부터 제거)
	if len(logBuffer) > maxLogLines {
		logBuffer = logBuffer[len(logBuffer)-maxLogLines:]
	}

	// 버퍼 전체를 하나의 텍스트로 합침
	displayText := strings.Join(logBuffer, "\n")
	logMutex.Unlock()

	cText := C.CString(displayText)
	C.UpdateStatus(cText)
	C.free(unsafe.Pointer(cText))
	// 여러 번 이벤트 처리하여 UI 업데이트 보장
	pumpEvents()
	time.Sleep(10 * time.Millisecond)
	pumpEvents()
}

// ClearLog 로그 버퍼 초기화
func ClearLog() {
	logMutex.Lock()
	logBuffer = nil
	logMutex.Unlock()
}

// ShowStatusOnly 상태 패널 + 채팅/입력 영역 오버레이 표시 (클립보드 모드용)
// chatW, chatH: 채팅 영역 크기 (380 x 430)
// inputW, inputH: 입력 영역 크기 (380 x 50)
// clickX, clickY: 입력창 왼쪽 상단에서 20,20 떨어진 클릭 좌표
// chatOffsetY: 사용하지 않음 (호환성 유지)
func ShowStatusOnly(clickX, clickY int, chatOffsetY int, chatW, chatH, inputW, inputH int) {
	if !initialized {
		Init()
	}
	ClearLog() // 새 세션 시작 시 로그 버퍼 초기화

	// 입력 영역 위치 (초록색) - 클릭 좌표는 입력창 왼쪽 상단에서 20,20 떨어진 곳
	inputX := clickX - 20
	inputY := clickY - 20

	// 채팅 영역 위치 (빨간색) - 입력 영역 바로 위에 2픽셀 간격으로 배치
	chatX := inputX // 입력 영역과 왼쪽 정렬
	chatY := inputY - 2 - chatH // 입력 영역 상단에서 2픽셀 위로

	// 상태 패널 크기와 위치 (채팅 영역 오른쪽, 높이 430)
	statusW := 280
	statusH := 430 // 고정 높이 430
	statusX := chatX + chatW + 10
	statusY := chatY

	// 화면 경계 체크 (최소 50픽셀 여백 유지)
	if chatX < 50 {
		chatX = 50
		inputX = 50
	}
	if chatY < 50 {
		chatY = 50
		inputY = chatY + chatH + 2
	}

	// 채팅 영역 표시 (빨간색)
	C.ShowOCRRegion(C.int(chatX), C.int(chatY), C.int(chatW), C.int(chatH))

	// 입력 영역 표시 (초록색)
	C.ShowInputRegion(C.int(inputX), C.int(inputY), C.int(inputW), C.int(inputH))

	// 상태 패널 표시
	C.ShowStatusPanel(C.int(statusX), C.int(statusY), C.int(statusW), C.int(statusH))

	// 단축키 안내 패널 (입력 영역 아래)
	infoX := inputX
	infoY := inputY + inputH + 5
	cText := C.CString("⌨ F9: 종료")
	C.ShowInfoPanel(C.int(infoX), C.int(infoY), cText)
	C.free(unsafe.Pointer(cText))

	// 이벤트 처리
	pumpEvents()
	time.Sleep(150 * time.Millisecond)
	pumpEvents()
}

// PumpEvents Cocoa 이벤트 루프 펌핑 (외부에서 호출용)
// waitForResponse 등 장시간 대기 중에도 버튼 클릭 이벤트를 처리하기 위해 사용
func PumpEvents() {
	if !initialized {
		return
	}
	pumpEvents()
}

// HideAll 모든 오버레이 숨기기
func HideAll() {
	C.HideAllOverlays()
	ClearLog() // 로그 버퍼 초기화
	pumpEvents()
}

// ShowForDuration 지정 시간 동안 오버레이 표시
func ShowForDuration(x, y, width, height int, duration time.Duration) {
	Show(x, y, width, height)
	time.Sleep(duration)
	Hide()
}

// ShowControlPanel 하위 호환용 (미사용)
func ShowControlPanel(x, y int) {}

// HideControlPanel 하위 호환용 (미사용)
func HideControlPanel() {
	C.HideControlPanel()
	pumpEvents()
}

// ShowInfoPanel 단축키 안내 패널 표시 (초록 테두리 아래)
func ShowInfoPanel(x, y int, text string) {
	if !initialized {
		Init()
	}
	cText := C.CString(text)
	C.ShowInfoPanel(C.int(x), C.int(y), cText)
	C.free(unsafe.Pointer(cText))
	pumpEvents()
}

// CheckPauseClicked 일시정지 버튼 클릭 확인
func CheckPauseClicked() bool {
	pumpEvents()
	result := C.CheckPauseClicked()
	return result != 0
}

// CheckStopClicked 종료 버튼 클릭 확인
func CheckStopClicked() bool {
	pumpEvents()
	result := C.CheckStopClicked()
	return result != 0
}

// CheckRestartClicked 재시작 버튼 클릭 확인
func CheckRestartClicked() bool {
	pumpEvents()
	result := C.CheckRestartClicked()
	return result != 0
}
