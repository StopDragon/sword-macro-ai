#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
SwordMacro 빌드 스크립트

Usage:
    python build.py          # 현재 플랫폼용 빌드
    python build.py --clean  # 빌드 디렉토리 정리 후 빌드
    python build.py --dmg    # macOS: DMG 생성까지 (create-dmg 필요)

Requirements:
    pip install pyinstaller

macOS DMG 생성 (선택):
    brew install create-dmg
"""

import os
import sys
import shutil
import subprocess
import argparse

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
IS_MAC = sys.platform == 'darwin'
IS_WIN = sys.platform == 'win32'

APP_NAME = 'SwordMacro'
VERSION = '1.0.0'


def clean_build():
    """빌드 아티팩트 정리"""
    dirs_to_remove = ['build', 'dist', '__pycache__']
    files_to_remove = [f for f in os.listdir(SCRIPT_DIR) if f.endswith('.pyc')]

    for d in dirs_to_remove:
        path = os.path.join(SCRIPT_DIR, d)
        if os.path.exists(path):
            print(f"🗑️  삭제: {d}/")
            shutil.rmtree(path)

    for f in files_to_remove:
        path = os.path.join(SCRIPT_DIR, f)
        if os.path.exists(path):
            os.remove(path)

    print("✅ 정리 완료\n")


def check_pyinstaller():
    """PyInstaller 설치 확인"""
    try:
        import PyInstaller
        print(f"✅ PyInstaller {PyInstaller.__version__} 감지됨")
        return True
    except ImportError:
        print("❌ PyInstaller가 설치되지 않았습니다.")
        print("   pip install pyinstaller")
        return False


def build_mac():
    """macOS 빌드 (.app 번들)"""
    spec_file = os.path.join(SCRIPT_DIR, 'sword_macro_mac.spec')

    if not os.path.exists(spec_file):
        print(f"❌ spec 파일을 찾을 수 없습니다: {spec_file}")
        return False

    print("🔨 macOS 빌드 시작...")
    print(f"   Spec: {spec_file}")
    print()

    result = subprocess.run(
        [sys.executable, '-m', 'PyInstaller', '--clean', spec_file],
        cwd=SCRIPT_DIR
    )

    if result.returncode == 0:
        app_path = os.path.join(SCRIPT_DIR, 'dist', f'{APP_NAME}.app')
        if os.path.exists(app_path):
            print()
            print("=" * 50)
            print(f"✅ 빌드 성공!")
            print(f"   📦 {app_path}")
            print()
            print("📋 다음 단계:")
            print("   1. dist/SwordMacro.app을 /Applications로 이동")
            print("   2. 시스템 설정 > 접근성에서 앱 허용")
            print("=" * 50)
            return True

    print("❌ 빌드 실패")
    return False


def build_windows():
    """Windows 빌드 (.exe)"""
    spec_file = os.path.join(SCRIPT_DIR, 'sword_macro_win.spec')

    if not os.path.exists(spec_file):
        print(f"❌ spec 파일을 찾을 수 없습니다: {spec_file}")
        return False

    print("🔨 Windows 빌드 시작...")
    print(f"   Spec: {spec_file}")
    print()
    print("⚠️  EasyOCR/PyTorch 포함으로 시간이 오래 걸릴 수 있습니다...")
    print()

    result = subprocess.run(
        [sys.executable, '-m', 'PyInstaller', '--clean', spec_file],
        cwd=SCRIPT_DIR
    )

    if result.returncode == 0:
        exe_path = os.path.join(SCRIPT_DIR, 'dist', APP_NAME, f'{APP_NAME}.exe')
        if os.path.exists(exe_path):
            # 폴더 크기 계산
            total_size = 0
            dist_dir = os.path.join(SCRIPT_DIR, 'dist', APP_NAME)
            for dirpath, dirnames, filenames in os.walk(dist_dir):
                for f in filenames:
                    fp = os.path.join(dirpath, f)
                    total_size += os.path.getsize(fp)
            size_mb = total_size / (1024 * 1024)

            print()
            print("=" * 50)
            print(f"✅ 빌드 성공!")
            print(f"   📦 {exe_path}")
            print(f"   📊 크기: {size_mb:.1f} MB")
            print()
            print("📋 배포 방법:")
            print(f"   dist/{APP_NAME}/ 폴더 전체를 ZIP으로 압축하여 배포")
            print()
            print("⚠️  주의:")
            print("   첫 실행 시 한국어 OCR 모델 다운로드 필요 (~200MB)")
            print("=" * 50)
            return True

    print("❌ 빌드 실패")
    return False


def create_dmg():
    """macOS DMG 생성 (create-dmg 필요)"""
    app_path = os.path.join(SCRIPT_DIR, 'dist', f'{APP_NAME}.app')
    dmg_path = os.path.join(SCRIPT_DIR, 'dist', f'{APP_NAME}-{VERSION}.dmg')

    if not os.path.exists(app_path):
        print(f"❌ .app 번들을 찾을 수 없습니다: {app_path}")
        print("   먼저 빌드를 실행하세요.")
        return False

    # create-dmg 확인
    if shutil.which('create-dmg') is None:
        print("❌ create-dmg가 설치되지 않았습니다.")
        print("   brew install create-dmg")
        return False

    # 기존 DMG 삭제
    if os.path.exists(dmg_path):
        os.remove(dmg_path)

    print("📀 DMG 생성 중...")

    result = subprocess.run([
        'create-dmg',
        '--volname', f'{APP_NAME} {VERSION}',
        '--volicon', app_path + '/Contents/Resources/icon.icns' if os.path.exists(app_path + '/Contents/Resources/icon.icns') else '',
        '--window-pos', '200', '120',
        '--window-size', '600', '400',
        '--icon-size', '100',
        '--icon', f'{APP_NAME}.app', '175', '190',
        '--app-drop-link', '425', '190',
        '--hide-extension', f'{APP_NAME}.app',
        dmg_path,
        app_path
    ], cwd=SCRIPT_DIR)

    if result.returncode == 0 and os.path.exists(dmg_path):
        size_mb = os.path.getsize(dmg_path) / (1024 * 1024)
        print()
        print("=" * 50)
        print(f"✅ DMG 생성 완료!")
        print(f"   📀 {dmg_path}")
        print(f"   📊 크기: {size_mb:.1f} MB")
        print("=" * 50)
        return True

    print("❌ DMG 생성 실패")
    return False


def main():
    parser = argparse.ArgumentParser(description='SwordMacro 빌드 스크립트')
    parser.add_argument('--clean', action='store_true', help='빌드 전 정리')
    parser.add_argument('--dmg', action='store_true', help='macOS: DMG 생성까지')
    args = parser.parse_args()

    os.chdir(SCRIPT_DIR)

    print()
    print("=" * 50)
    print(f"  🗡️  SwordMacro 빌드 스크립트 v{VERSION}")
    print(f"  📍 플랫폼: {'macOS' if IS_MAC else 'Windows' if IS_WIN else 'Linux'}")
    print("=" * 50)
    print()

    if args.clean:
        clean_build()

    if not check_pyinstaller():
        sys.exit(1)

    print()

    if IS_MAC:
        success = build_mac()
        if success and args.dmg:
            print()
            create_dmg()
    elif IS_WIN:
        build_windows()
    else:
        print("❌ 지원되지 않는 플랫폼입니다.")
        print("   macOS 또는 Windows에서 실행하세요.")
        sys.exit(1)


if __name__ == '__main__':
    main()
