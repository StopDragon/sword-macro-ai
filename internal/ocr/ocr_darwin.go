//go:build darwin

package ocr

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Vision -framework CoreGraphics -framework Foundation

#import <Vision/Vision.h>
#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>

char* recognizeText(unsigned char* pixelData, int width, int height) {
    @autoreleasepool {
        // RGBA 데이터로 CGImage 생성
        CGColorSpaceRef colorSpace = CGColorSpaceCreateDeviceRGB();
        CGContextRef context = CGBitmapContextCreate(
            pixelData,
            width,
            height,
            8,
            width * 4,
            colorSpace,
            kCGImageAlphaPremultipliedLast
        );

        if (context == NULL) {
            CGColorSpaceRelease(colorSpace);
            return strdup("");
        }

        CGImageRef cgImage = CGBitmapContextCreateImage(context);
        CGContextRelease(context);
        CGColorSpaceRelease(colorSpace);

        if (cgImage == NULL) {
            return strdup("");
        }

        // Vision OCR 요청 생성
        __block NSMutableString* resultText = [NSMutableString string];
        dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);

        VNRecognizeTextRequest* request = [[VNRecognizeTextRequest alloc]
            initWithCompletionHandler:^(VNRequest* request, NSError* error) {
                if (error == nil && request.results != nil) {
                    NSArray* results = request.results;

                    // y 좌표로 정렬 (위→아래)
                    NSArray* sorted = [results sortedArrayUsingComparator:
                        ^NSComparisonResult(VNRecognizedTextObservation* a, VNRecognizedTextObservation* b) {
                            CGFloat ay = 1.0 - a.boundingBox.origin.y;
                            CGFloat by = 1.0 - b.boundingBox.origin.y;
                            if (ay < by) return NSOrderedAscending;
                            if (ay > by) return NSOrderedDescending;
                            return NSOrderedSame;
                        }];

                    for (VNRecognizedTextObservation* observation in sorted) {
                        VNRecognizedText* topCandidate = [[observation topCandidates:1] firstObject];
                        if (topCandidate != nil) {
                            if ([resultText length] > 0) {
                                [resultText appendString:@"\n"];
                            }
                            [resultText appendString:topCandidate.string];
                        }
                    }
                }
                dispatch_semaphore_signal(semaphore);
            }];

        request.recognitionLanguages = @[@"ko", @"en"];
        request.recognitionLevel = VNRequestTextRecognitionLevelAccurate;

        // 요청 실행
        VNImageRequestHandler* handler = [[VNImageRequestHandler alloc]
            initWithCGImage:cgImage options:@{}];

        NSError* error = nil;
        [handler performRequests:@[request] error:&error];

        // 완료 대기
        dispatch_semaphore_wait(semaphore, DISPATCH_TIME_FOREVER);

        CGImageRelease(cgImage);

        return strdup([resultText UTF8String]);
    }
}

void freeString(char* str) {
    if (str != NULL) {
        free(str);
    }
}
*/
import "C"

import (
	"image"
	"unsafe"
)

type visionEngine struct{}

func newEngine() Engine {
	return &visionEngine{}
}

func (e *visionEngine) Init() error {
	// Vision Framework는 초기화 필요 없음
	return nil
}

func (e *visionEngine) Recognize(img *image.RGBA) (string, error) {
	if img == nil {
		return "", nil
	}

	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	// Go 이미지 데이터를 C로 전달
	result := C.recognizeText(
		(*C.uchar)(unsafe.Pointer(&img.Pix[0])),
		C.int(width),
		C.int(height),
	)
	defer C.freeString(result)

	return C.GoString(result), nil
}

func (e *visionEngine) Close() {
	// 정리 필요 없음
}
