# -*- coding: utf-8 -*-
"""플랫폼별 화면 캡처 추상화

macOS: Quartz CGWindowListCreateImage (기존 방식 유지)
Windows/Linux: mss 라이브러리
"""
import sys

IS_MAC = sys.platform == 'darwin'

if IS_MAC:
    import Quartz
    from PIL import Image
    import io

    def capture_region(x, y, w, h):
        """macOS: Quartz로 화면 영역 캡처 → PIL Image 반환"""
        rect = Quartz.CGRectMake(x, y, w, h)
        cg_img = Quartz.CGWindowListCreateImage(
            rect,
            Quartz.kCGWindowListOptionOnScreenOnly,
            Quartz.kCGNullWindowID,
            Quartz.kCGWindowImageDefault
        )
        if cg_img is None:
            return None

        # CGImage → PIL Image 변환
        width = Quartz.CGImageGetWidth(cg_img)
        height = Quartz.CGImageGetHeight(cg_img)
        bytes_per_row = Quartz.CGImageGetBytesPerRow(cg_img)
        data_provider = Quartz.CGImageGetDataProvider(cg_img)
        data = Quartz.CGDataProviderCopyData(data_provider)

        img = Image.frombytes('RGBA', (width, height), data, 'raw', 'BGRA', bytes_per_row)
        return img

else:
    import mss
    from PIL import Image

    _sct = mss.mss()

    def capture_region(x, y, w, h):
        """Windows/Linux: mss로 화면 영역 캡처 → PIL Image 반환"""
        monitor = {'left': x, 'top': y, 'width': w, 'height': h}
        screenshot = _sct.grab(monitor)
        img = Image.frombytes('RGB', screenshot.size, screenshot.bgra, 'raw', 'BGRX')
        return img
