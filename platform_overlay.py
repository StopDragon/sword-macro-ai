# -*- coding: utf-8 -*-
"""플랫폼별 오버레이 UI 추상화

macOS: AppKit NSWindow (기존 서브프로세스 방식)
Windows/Linux: tkinter (Python 내장, 별도 Thread)
"""
import sys
import os
import subprocess
import threading
import time
import tempfile

IS_MAC = sys.platform == 'darwin'

CAPTURE_W = 375
CAPTURE_H = 550
INPUT_BOX_H = 80
STATUS_FILE = os.path.join(tempfile.gettempdir(), 'sword_macro_status.txt')


# ─── macOS: AppKit 오버레이 (서브프로세스) ──────────
_OVERLAY_SCRIPT_MAC = '''\
import sys, AppKit, Quartz

x, y = int(sys.argv[1]), int(sys.argv[2])
ocr_w, ocr_h = int(sys.argv[3]), int(sys.argv[4])
input_h = int(sys.argv[5])
status_file = sys.argv[6]
pad = 4

app = AppKit.NSApplication.sharedApplication()
app.setActivationPolicy_(AppKit.NSApplicationActivationPolicyAccessory)
screen_h = AppKit.NSScreen.mainScreen().frame().size.height

def make_overlay(rx, ry, rw, rh, r, g, b):
    flipped_y = screen_h - ry - rh
    rect = AppKit.NSMakeRect(rx, flipped_y, rw, rh)
    win = AppKit.NSWindow.alloc().initWithContentRect_styleMask_backing_defer_(
        rect, AppKit.NSWindowStyleMaskBorderless, AppKit.NSBackingStoreBuffered, False)
    win.setLevel_(AppKit.NSFloatingWindowLevel)
    win.setBackgroundColor_(AppKit.NSColor.clearColor())
    win.setOpaque_(False)
    win.setIgnoresMouseEvents_(True)
    win.setHasShadow_(False)
    view = AppKit.NSView.alloc().initWithFrame_(rect)
    view.setWantsLayer_(True)
    view.layer().setBorderColor_(Quartz.CGColorCreateGenericRGB(r, g, b, 0.8))
    view.layer().setBorderWidth_(2.0)
    view.layer().setBackgroundColor_(Quartz.CGColorCreateGenericRGB(0, 0, 0, 0))
    win.setContentView_(view)
    win.orderFront_(None)
    return win

w1 = make_overlay(x - pad, y - pad, ocr_w + pad*2, ocr_h + pad*2, 0.0, 1.0, 0.0)
w2 = make_overlay(x - pad, y + ocr_h + pad, ocr_w + pad*2, input_h, 1.0, 0.2, 0.2)

panel_w = 300
panel_h = ocr_h + pad * 2
panel_x = x + ocr_w + pad + 8
panel_y = y - pad
flipped_py = screen_h - panel_y - panel_h
panel_rect = AppKit.NSMakeRect(panel_x, flipped_py, panel_w, panel_h)
panel_win = AppKit.NSWindow.alloc().initWithContentRect_styleMask_backing_defer_(
    panel_rect, AppKit.NSWindowStyleMaskBorderless, AppKit.NSBackingStoreBuffered, False)
panel_win.setLevel_(AppKit.NSFloatingWindowLevel)
panel_win.setBackgroundColor_(AppKit.NSColor.colorWithCalibratedRed_green_blue_alpha_(0.1, 0.1, 0.1, 0.85))
panel_win.setOpaque_(False)
panel_win.setIgnoresMouseEvents_(True)
panel_win.setHasShadow_(True)

label = AppKit.NSTextField.alloc().initWithFrame_(AppKit.NSMakeRect(8, 4, panel_w - 16, panel_h - 8))
label.setBezeled_(False)
label.setDrawsBackground_(False)
label.setEditable_(False)
label.setSelectable_(False)
label.setTextColor_(AppKit.NSColor.colorWithCalibratedRed_green_blue_alpha_(0.0, 1.0, 0.4, 1.0))
label.setFont_(AppKit.NSFont.monospacedSystemFontOfSize_weight_(11, AppKit.NSFontWeightMedium))
label.setMaximumNumberOfLines_(30)
label.setStringValue_("대기 중...")
panel_win.contentView().addSubview_(label)
panel_win.orderFront_(None)

_prev_text = [""]
import threading
def poll_loop():
    import time as _t
    while True:
        _t.sleep(0.3)
        try:
            with open(status_file, "r", encoding="utf-8") as f:
                text = f.read().strip()
            if text and text != _prev_text[0]:
                label.performSelectorOnMainThread_withObject_waitUntilDone_(
                    "setStringValue:", text, False)
                _prev_text[0] = text
        except:
            pass

t = threading.Thread(target=poll_loop, daemon=True)
t.start()

app.run()
'''


# ─── tkinter 오버레이 (Windows/Linux) ──────────────
class _TkOverlay:
    """tkinter 기반 크로스플랫폼 오버레이"""

    def __init__(self):
        self._thread = None
        self._running = False
        self._root = None

    def show(self, x, y, w, h):
        self.hide()
        self._running = True
        self._thread = threading.Thread(
            target=self._run, args=(x, y, w, h), daemon=True)
        self._thread.start()

    def _run(self, x, y, w, h):
        import tkinter as tk

        pad = 4
        root = tk.Tk()
        root.withdraw()
        self._root = root

        # OCR 영역 (초록 테두리)
        ocr_win = tk.Toplevel(root)
        ocr_win.overrideredirect(True)
        ocr_win.attributes('-topmost', True)
        ocr_win.geometry(f'{w + pad * 2}x{h + pad * 2}+{x - pad}+{y - pad}')
        if sys.platform == 'win32':
            ocr_win.attributes('-transparentcolor', 'black')
        ocr_win.configure(bg='black')
        ocr_frame = tk.Frame(ocr_win, bg='black',
                             highlightbackground='#00ff00',
                             highlightthickness=2)
        ocr_frame.pack(fill='both', expand=True)

        # 입력창 영역 (빨간 테두리)
        input_win = tk.Toplevel(root)
        input_win.overrideredirect(True)
        input_win.attributes('-topmost', True)
        input_win.geometry(f'{w + pad * 2}x{INPUT_BOX_H}+{x - pad}+{y + h + pad}')
        if sys.platform == 'win32':
            input_win.attributes('-transparentcolor', 'black')
        input_win.configure(bg='black')
        input_frame = tk.Frame(input_win, bg='black',
                               highlightbackground='#ff3333',
                               highlightthickness=2)
        input_frame.pack(fill='both', expand=True)

        # 상태 패널
        panel_w = 300
        panel_h = h + pad * 2
        panel_x = x + w + pad + 8
        panel_y = y - pad

        panel_win = tk.Toplevel(root)
        panel_win.overrideredirect(True)
        panel_win.attributes('-topmost', True)
        panel_win.geometry(f'{panel_w}x{panel_h}+{panel_x}+{panel_y}')
        panel_win.configure(bg='#1a1a1a')

        label = tk.Label(
            panel_win, text="대기 중...",
            fg='#00ff66', bg='#1a1a1a',
            font=('Consolas' if sys.platform == 'win32' else 'Menlo', 10),
            anchor='nw', justify='left',
            wraplength=panel_w - 16
        )
        label.pack(fill='both', expand=True, padx=8, pady=4)

        # 상태 파일 폴링
        prev_text = [""]

        def poll_status():
            if not self._running:
                root.quit()
                return
            try:
                with open(STATUS_FILE, 'r', encoding='utf-8') as f:
                    text = f.read().strip()
                if text and text != prev_text[0]:
                    label.config(text=text)
                    prev_text[0] = text
            except Exception:
                pass
            root.after(300, poll_status)

        root.after(300, poll_status)
        root.mainloop()

    def hide(self):
        self._running = False
        if self._root:
            try:
                self._root.quit()
            except Exception:
                pass
            self._root = None

    def show_at(self, click_x, click_y):
        input_top = click_y - INPUT_BOX_H // 2
        ox = max(click_x - CAPTURE_W // 2, 0)
        oy = input_top - CAPTURE_H
        self.show(ox, oy, CAPTURE_W, CAPTURE_H)


# ─── macOS AppKit 오버레이 (서브프로세스) ───────────
class _MacOverlay:
    """macOS 전용 AppKit 오버레이 (기존 방식)"""

    def __init__(self):
        self._proc = None

    def show(self, x, y, w, h):
        self.hide()
        with open(STATUS_FILE, 'w', encoding='utf-8') as f:
            f.write("대기 중...")
        self._proc = subprocess.Popen(
            [sys.executable, '-c', _OVERLAY_SCRIPT_MAC,
             str(x), str(y), str(w), str(h), str(INPUT_BOX_H), STATUS_FILE],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
        )

    def hide(self):
        if self._proc and self._proc.poll() is None:
            self._proc.terminate()
            self._proc = None

    def show_at(self, click_x, click_y):
        input_top = click_y - INPUT_BOX_H // 2
        ox = max(click_x - CAPTURE_W // 2, 0)
        oy = input_top - CAPTURE_H
        self.show(ox, oy, CAPTURE_W, CAPTURE_H)


# ─── 팩토리 ────────────────────────────────────────
def create_overlay():
    """플랫폼에 맞는 오버레이 인스턴스 반환"""
    if IS_MAC:
        return _MacOverlay()
    return _TkOverlay()
