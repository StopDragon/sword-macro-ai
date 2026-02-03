# -*- coding: utf-8 -*-
"""플랫폼별 OCR 엔진 추상화

macOS: Apple Vision Framework (빠름, 정확, 추가 설치 불필요)
Windows/Linux: EasyOCR (PyTorch 기반, 첫 실행 시 모델 다운로드 ~200MB)
"""
import sys

IS_MAC = sys.platform == 'darwin'

_engine = None


def init_ocr():
    """OCR 엔진 초기화. 앱 시작 시 1회 호출."""
    global _engine

    if IS_MAC:
        import Vision
        req = Vision.VNRecognizeTextRequest.alloc().init()
        req.setRecognitionLanguages_(['ko', 'en'])
        req.setRecognitionLevel_(Vision.VNRequestTextRecognitionLevelAccurate)
        _engine = ('vision', req)
    else:
        import easyocr
        print("[OCR] EasyOCR 초기화 중... (첫 실행 시 모델 다운로드가 필요합니다)")
        reader = easyocr.Reader(['ko', 'en'], gpu=True, verbose=False)
        print("[OCR] EasyOCR 준비 완료")
        _engine = ('easyocr', reader)


def recognize_text(image):
    """이미지에서 텍스트 인식. 위→아래 순서로 줄 단위 반환.

    Args:
        image: PIL.Image 또는 macOS CGImage
    Returns:
        str: 인식된 텍스트 (줄바꿈 구분)
    """
    if _engine is None:
        init_ocr()

    engine_type, engine = _engine

    if engine_type == 'vision':
        return _recognize_vision(image, engine)
    else:
        return _recognize_easyocr(image, engine)


def _recognize_vision(pil_image, ocr_req):
    """macOS Vision Framework로 텍스트 인식"""
    import Vision
    import Quartz
    from PIL import Image

    # PIL Image → CGImage 변환
    # RGBA로 변환 후 raw bytes에서 CGImage 생성
    if pil_image.mode != 'RGBA':
        pil_image = pil_image.convert('RGBA')

    width, height = pil_image.size
    raw_data = pil_image.tobytes('raw', 'RGBA')

    color_space = Quartz.CGColorSpaceCreateDeviceRGB()
    data_provider = Quartz.CGDataProviderCreateWithData(None, raw_data, len(raw_data), None)
    cg_img = Quartz.CGImageCreate(
        width, height,
        8, 32, width * 4,
        color_space,
        Quartz.kCGImageAlphaPremultipliedLast,
        data_provider, None, False,
        Quartz.kCGRenderingIntentDefault
    )

    if cg_img is None:
        return ""

    handler = Vision.VNImageRequestHandler.alloc().initWithCGImage_options_(cg_img, None)
    ok, _ = handler.performRequests_error_([ocr_req], None)
    if not ok:
        return ""

    results = ocr_req.results()
    if not results:
        return ""

    lines = []
    for r in results:
        text = r.topCandidates_(1)[0].string()
        y_pos = 1.0 - r.boundingBox().origin.y
        lines.append((y_pos, text))
    lines.sort(key=lambda p: p[0])
    return '\n'.join(t for _, t in lines)


def _recognize_easyocr(pil_image, reader):
    """EasyOCR로 텍스트 인식"""
    import numpy as np

    # PIL Image → numpy array
    if pil_image.mode == 'RGBA':
        pil_image = pil_image.convert('RGB')
    img_array = np.array(pil_image)

    results = reader.readtext(img_array, detail=1)
    if not results:
        return ""

    # bbox 기준 위→아래 정렬 (y좌표 기준)
    results.sort(key=lambda r: r[0][0][1])  # top-left y 좌표
    return '\n'.join(text for _, text, _ in results)
