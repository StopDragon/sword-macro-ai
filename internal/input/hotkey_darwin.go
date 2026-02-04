//go:build darwin

package input

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework Carbon

#include <CoreGraphics/CoreGraphics.h>
#include <Carbon/Carbon.h>
#include <pthread.h>

// 핫키 눌림 플래그 (Go에서 폴링)
static volatile int f8Pressed = 0;
static volatile int f9Pressed = 0;

static CFMachPortRef eventTap = NULL;
static CFRunLoopRef tapRunLoop = NULL;
static CFRunLoopSourceRef tapSource = NULL;
static pthread_t tapThread;
static volatile int tapRunning = 0;

// CGEventTap 콜백: F8/F9 감지
CGEventRef hotkeyTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    // 탭이 비활성화되면 재활성화
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (eventTap) {
            CGEventTapEnable(eventTap, true);
        }
        return event;
    }

    if (type == kCGEventKeyDown) {
        CGKeyCode keycode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
        if (keycode == kVK_F8) {  // 0x64 = 100
            f8Pressed = 1;
            return NULL; // 이벤트 소비 (다른 앱에 전달 안함)
        }
        if (keycode == kVK_F9) {  // 0x65 = 101
            f9Pressed = 1;
            return NULL;
        }
    }

    return event; // 다른 키는 그대로 전달
}

// 핫키 리스닝 스레드 함수
void* hotkeyThreadFunc(void* arg) {
    @autoreleasepool {
        eventTap = CGEventTapCreate(
            kCGSessionEventTap,
            kCGHeadInsertEventTap,
            kCGEventTapOptionDefault,  // 이벤트 소비 가능
            CGEventMaskBit(kCGEventKeyDown),
            hotkeyTapCallback,
            NULL
        );

        if (!eventTap) {
            // 접근성 권한 없음 → 조용히 실패
            tapRunning = 0;
            return NULL;
        }

        tapSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, eventTap, 0);
        tapRunLoop = CFRunLoopGetCurrent();
        CFRunLoopAddSource(tapRunLoop, tapSource, kCFRunLoopDefaultMode);
        CGEventTapEnable(eventTap, true);

        tapRunning = 1;
        CFRunLoopRun(); // 블로킹 - StopHotkeyListener()가 CFRunLoopStop 호출할 때까지

        // 정리
        CGEventTapEnable(eventTap, false);
        CFRunLoopRemoveSource(tapRunLoop, tapSource, kCFRunLoopDefaultMode);
        CFRelease(tapSource);
        CFRelease(eventTap);
        eventTap = NULL;
        tapSource = NULL;
        tapRunLoop = NULL;
        tapRunning = 0;
    }
    return NULL;
}

// 핫키 리스닝 시작 (백그라운드 스레드)
void StartHotkeyListenerC() {
    if (tapRunning) return;
    f8Pressed = 0;
    f9Pressed = 0;
    pthread_create(&tapThread, NULL, hotkeyThreadFunc, NULL);
}

// 핫키 리스닝 중지
void StopHotkeyListenerC() {
    if (tapRunLoop) {
        CFRunLoopStop(tapRunLoop);
    }
    tapRunning = 0;
}

// F8 눌림 확인 (확인 후 플래그 리셋)
int CheckF8PressedC() {
    if (f8Pressed) {
        f8Pressed = 0;
        return 1;
    }
    return 0;
}

// F9 눌림 확인 (확인 후 플래그 리셋)
int CheckF9PressedC() {
    if (f9Pressed) {
        f9Pressed = 0;
        return 1;
    }
    return 0;
}

int IsHotkeyListenerRunningC() {
    return tapRunning;
}
*/
import "C"
import (
	"fmt"
	"time"
)

// startPlatformHotkeys CGEventTap 기반 글로벌 핫키 리스닝 시작
func startPlatformHotkeys() {
	C.StartHotkeyListenerC()
	// 리스너 시작 대기 (스레드 생성 시간)
	time.Sleep(100 * time.Millisecond)
	if C.IsHotkeyListenerRunningC() != 0 {
		fmt.Println("✅ 핫키 리스너 활성화 (F8: 일시정지, F9: 종료)")
	} else {
		fmt.Println("⚠️  핫키 리스너 실패 (접근성 권한 필요: 시스템 설정 → 개인정보 보호 → 접근성)")
		fmt.Println("   오버레이 버튼으로 조작 가능합니다.")
	}
}

// stopPlatformHotkeys 핫키 리스닝 중지
func stopPlatformHotkeys() {
	C.StopHotkeyListenerC()
}

// checkF8 F8 키 눌림 확인
func checkF8() bool {
	return C.CheckF8PressedC() != 0
}

// checkF9 F9 키 눌림 확인
func checkF9() bool {
	return C.CheckF9PressedC() != 0
}
