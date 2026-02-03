//go:build darwin

package capture

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreGraphics -framework CoreMedia -framework Foundation

#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreMedia/CoreMedia.h>
#import <Foundation/Foundation.h>

typedef struct {
    unsigned char* data;
    int width;
    int height;
    int bytesPerRow;
    int error;
} CaptureResult;

CaptureResult captureScreenRegion(int x, int y, int width, int height) {
    __block CaptureResult result = {NULL, 0, 0, 0, 0};

    dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);

    // 공유 가능한 콘텐츠 가져오기
    [SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent* content, NSError* error) {
        if (error || content.displays.count == 0) {
            result.error = 1;
            dispatch_semaphore_signal(semaphore);
            return;
        }

        SCDisplay* display = content.displays[0];

        // 스트림 설정
        SCContentFilter* filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
        SCStreamConfiguration* config = [[SCStreamConfiguration alloc] init];

        // 캡처 영역 설정
        config.sourceRect = CGRectMake(x, y, width, height);
        config.width = width;
        config.height = height;
        config.pixelFormat = kCVPixelFormatType_32BGRA;
        config.showsCursor = NO;

        // 단일 프레임 캡처
        [SCScreenshotManager captureImageWithFilter:filter
                                     configuration:config
                                 completionHandler:^(CGImageRef image, NSError* error) {
            if (error || image == NULL) {
                result.error = 2;
                dispatch_semaphore_signal(semaphore);
                return;
            }

            result.width = (int)CGImageGetWidth(image);
            result.height = (int)CGImageGetHeight(image);
            result.bytesPerRow = (int)CGImageGetBytesPerRow(image);

            CFDataRef dataRef = CGDataProviderCopyData(CGImageGetDataProvider(image));
            if (dataRef == NULL) {
                result.error = 3;
                dispatch_semaphore_signal(semaphore);
                return;
            }

            long length = CFDataGetLength(dataRef);
            result.data = (unsigned char*)malloc(length);
            if (result.data != NULL) {
                memcpy(result.data, CFDataGetBytePtr(dataRef), length);
            }

            CFRelease(dataRef);
            dispatch_semaphore_signal(semaphore);
        }];
    }];

    // 타임아웃 5초
    dispatch_semaphore_wait(semaphore, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));

    return result;
}

void freeCaptureData(unsigned char* data) {
    if (data != NULL) {
        free(data);
    }
}
*/
import "C"

import (
	"errors"
	"image"
	"unsafe"
)

func captureRegion(x, y, width, height int) (*image.RGBA, error) {
	result := C.captureScreenRegion(C.int(x), C.int(y), C.int(width), C.int(height))

	if result.error != 0 {
		return nil, errors.New("화면 캡처 실패 (ScreenCaptureKit)")
	}

	if result.data == nil {
		return nil, errors.New("캡처 데이터 없음")
	}
	defer C.freeCaptureData(result.data)

	w := int(result.width)
	h := int(result.height)
	bytesPerRow := int(result.bytesPerRow)

	// BGRA → RGBA 변환
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	dataSlice := C.GoBytes(unsafe.Pointer(result.data), C.int(bytesPerRow*h))

	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			srcIdx := row*bytesPerRow + col*4
			dstIdx := row*img.Stride + col*4

			if srcIdx+3 < len(dataSlice) {
				// BGRA → RGBA
				img.Pix[dstIdx+0] = dataSlice[srcIdx+2] // R
				img.Pix[dstIdx+1] = dataSlice[srcIdx+1] // G
				img.Pix[dstIdx+2] = dataSlice[srcIdx+0] // B
				img.Pix[dstIdx+3] = dataSlice[srcIdx+3] // A
			}
		}
	}

	return img, nil
}
