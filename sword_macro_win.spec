# -*- mode: python ; coding: utf-8 -*-
"""
PyInstaller spec file for Windows (.exe)

Usage:
    pyinstaller sword_macro_win.spec

Output:
    dist/SwordMacro/SwordMacro.exe

Note:
    EasyOCR + PyTorch로 인해 최종 크기 500MB+ 예상
    첫 실행 시 한국어 OCR 모델 자동 다운로드됨 (~200MB)
"""

import sys
from PyInstaller.utils.hooks import collect_data_files, collect_submodules

block_cipher = None

# EasyOCR 및 PyTorch 관련 숨김 임포트
hiddenimports = [
    'easyocr',
    'torch',
    'torchvision',
    'cv2',
    'sklearn',
    'sklearn.utils._cython_blas',
    'sklearn.neighbors.typedefs',
    'sklearn.neighbors.quad_tree',
    'sklearn.tree._utils',
    'pynput.keyboard._win32',
    'pynput.mouse._win32',
]

# PIL/Pillow 플러그인
hiddenimports += collect_submodules('PIL')

# EasyOCR 데이터 파일 수집
datas = [
    ('README.md', '.'),
]

# EasyOCR 모델 경로 (선택적 - 모델 포함 시 크기 증가)
# 주석 해제하면 모델도 번들에 포함
# import easyocr
# import os
# easyocr_path = os.path.dirname(easyocr.__file__)
# datas += [(easyocr_path, 'easyocr')]

a = Analysis(
    ['sword_macro.py'],
    pathex=[],
    binaries=[],
    datas=datas,
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[
        # macOS 전용 패키지 제외
        'pyobjc',
        'objc',
        'Vision',
        'Quartz',
        'AppKit',
        'Foundation',
    ],
    win_no_prefer_redirects=False,
    win_private_assemblies=False,
    cipher=block_cipher,
    noarchive=False,
)

pyz = PYZ(a.pure, a.zipped_data, cipher=block_cipher)

exe = EXE(
    pyz,
    a.scripts,
    [],
    exclude_binaries=True,
    name='SwordMacro',
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    console=True,  # 콘솔 표시 (메뉴 입력 필요)
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
    icon=None,  # 아이콘 추가 시: 'icon.ico'
)

coll = COLLECT(
    exe,
    a.binaries,
    a.zipfiles,
    a.datas,
    strip=False,
    upx=True,
    upx_exclude=[],
    name='SwordMacro',
)
