//go:build windows

package capture

import (
	"errors"
	"image"
	"syscall"
	"unsafe"
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	gdi32                  = syscall.NewLazyDLL("gdi32.dll")
	getDC                  = user32.NewProc("GetDC")
	releaseDC              = user32.NewProc("ReleaseDC")
	createCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	selectObject           = gdi32.NewProc("SelectObject")
	bitBlt                 = gdi32.NewProc("BitBlt")
	deleteDC               = gdi32.NewProc("DeleteDC")
	deleteObject           = gdi32.NewProc("DeleteObject")
	getDIBits              = gdi32.NewProc("GetDIBits")
)

const (
	SRCCOPY = 0x00CC0020
	BI_RGB  = 0
)

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]uint32
}

func captureRegion(x, y, width, height int) (*image.RGBA, error) {
	hdcScreen, _, _ := getDC.Call(0)
	if hdcScreen == 0 {
		return nil, errors.New("GetDC 실패")
	}
	defer releaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := createCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return nil, errors.New("CreateCompatibleDC 실패")
	}
	defer deleteDC.Call(hdcMem)

	hBitmap, _, _ := createCompatibleBitmap.Call(hdcScreen, uintptr(width), uintptr(height))
	if hBitmap == 0 {
		return nil, errors.New("CreateCompatibleBitmap 실패")
	}
	defer deleteObject.Call(hBitmap)

	oldBitmap, _, _ := selectObject.Call(hdcMem, hBitmap)
	defer selectObject.Call(hdcMem, oldBitmap)

	ret, _, _ := bitBlt.Call(hdcMem, 0, 0, uintptr(width), uintptr(height),
		hdcScreen, uintptr(x), uintptr(y), SRCCOPY)
	if ret == 0 {
		return nil, errors.New("BitBlt 실패")
	}

	bmi := BITMAPINFO{
		BmiHeader: BITMAPINFOHEADER{
			BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
			BiWidth:       int32(width),
			BiHeight:      int32(-height),
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: BI_RGB,
		},
	}

	pixels := make([]byte, width*height*4)
	ret, _, _ = getDIBits.Call(hdcMem, hBitmap, 0, uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bmi)),
		0)
	if ret == 0 {
		return nil, errors.New("GetDIBits 실패")
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(pixels); i += 4 {
		img.Pix[i+0] = pixels[i+2] // R
		img.Pix[i+1] = pixels[i+1] // G
		img.Pix[i+2] = pixels[i+0] // B
		img.Pix[i+3] = pixels[i+3] // A
	}

	return img, nil
}
