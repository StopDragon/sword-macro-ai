# -*- mode: python ; coding: utf-8 -*-
"""
PyInstaller spec file for macOS (.app bundle → DMG)

Usage:
    pyinstaller sword_macro_mac.spec

Output:
    dist/SwordMacro.app
"""

import sys
from PyInstaller.utils.hooks import collect_data_files, collect_submodules

block_cipher = None

# PyObjC 서브모듈 수집 (Vision, Quartz, AppKit)
hiddenimports = [
    'pyobjc',
    'objc',
    'Vision',
    'Quartz',
    'Quartz.CoreGraphics',
    'AppKit',
    'Foundation',
    'pynput.keyboard._darwin',
    'pynput.mouse._darwin',
]

# PIL/Pillow 플러그인
hiddenimports += collect_submodules('PIL')

a = Analysis(
    ['sword_macro.py'],
    pathex=[],
    binaries=[],
    datas=[
        ('README.md', '.'),
    ],
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[
        # Windows/Linux 전용 패키지 제외
        'easyocr',
        'torch',
        'torchvision',
        'cv2',
        'numpy',  # easyocr 의존성
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
    console=False,  # GUI 앱 (콘솔 숨김)
    disable_windowed_traceback=False,
    argv_emulation=True,  # macOS argv 에뮬레이션
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
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

app = BUNDLE(
    coll,
    name='SwordMacro.app',
    icon=None,  # 아이콘 추가 시: 'icon.icns'
    bundle_identifier='com.sword-macro.ai',
    info_plist={
        'CFBundleName': 'SwordMacro',
        'CFBundleDisplayName': '검키우기 매크로',
        'CFBundleVersion': '1.0.0',
        'CFBundleShortVersionString': '1.0.0',
        'NSHighResolutionCapable': True,
        'NSAppleEventsUsageDescription': '자동화를 위해 접근성 권한이 필요합니다.',
        'NSAccessibilityUsageDescription': '키보드/마우스 자동화를 위해 접근성 권한이 필요합니다.',
    },
)
