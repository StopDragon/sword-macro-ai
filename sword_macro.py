# -*- coding: utf-8 -*-
import sys
import io
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')

import pyautogui
pyautogui.FAILSAFE = True
pyautogui.PAUSE = 0.05

import pyperclip
import time
import re
import json
import csv
import os
import atexit
import signal
from pynput import keyboard as pynput_keyboard

import platform_ocr
import platform_capture
from platform_overlay import create_overlay, STATUS_FILE, CAPTURE_W, CAPTURE_H, INPUT_BOX_H

# 플랫폼별 단축키 (macOS: command, Windows: ctrl)
_MOD_KEY = 'command' if sys.platform == 'darwin' else 'ctrl'

# ─── 상수 ───────────────────────────────────────
CMD_ENHANCE = '/강화'
CMD_SELL = '/판매'

MODE_TARGET = '1'
MODE_HIDDEN = '2'
MODE_MONEY = '3'

TRASH_ITEMS = ['낡은 검', '낡은 몽둥이', '낡은 도끼', '낡은 망치']

# 골드 채굴: ROI 최대 구간 고정 (+10 = ROI 2.90)
# 데이터 쌓이면 재조정
GOLD_MINE_TARGET = 10

# 딜레이 최적화 (6.5h 데이터 기반 조정)
# 기존: 1.5/1.8/2.7/4.5 → 평균 4.5초/턴, 62.6초/사이클
# 조정: 1.2/1.5/2.5/3.5 → 목표 ~3초/턴, ~55초/사이클
BOOST_LEVEL = 4        # 이 레벨 이하는 부스트 딜레이
BOOST_DELAY = 1.5      # 저강 부스트 딜레이 (초) ← 1.8
TRASH_DELAY = 1.2      # 트래시 판매→재강화 딜레이 (초) ← 1.5

RE_GOLD = re.compile(r'(?:남은 골드|현재 보유 골드):\s*([\d,]+)G')
RE_LEVEL = re.compile(r'\[\+(\d+)\]')

LOG_MAX_BYTES = 5 * 1024 * 1024

_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))


class Config:
    """설정 관리"""
    DEFAULTS = {
        'slow_start_level': 9,
        'fast_delay': 2.5,
        'slow_delay': 3.5,
        'min_gold': 0,
        'use_fixed_pos': False,
        'fixed_x': None,
        'fixed_y': None,
        'fixed_start_y': None,
        'clipboard_delay': 0.3,
        'input_delay': 0.12,
    }

    # 기존 설정 파일과의 키 매핑 (하위 호환)
    _LEGACY_MAP = {
        'SLOW_START_LEVEL': 'slow_start_level',
        'FAST_DELAY': 'fast_delay',
        'SLOW_DELAY': 'slow_delay',
        'MIN_GOLD_LIMIT': 'min_gold',
        'USE_CUSTOM_POS': 'use_fixed_pos',
        'FIXED_X': 'fixed_x',
        'FIXED_Y': 'fixed_y',
        'FIXED_START_Y': 'fixed_start_y',
        'CLIPBOARD_SAFETY_DELAY': 'clipboard_delay',
        'INPUT_DELAY': 'input_delay',
    }

    def __init__(self):
        self.path = os.path.join(_SCRIPT_DIR, 'sword_config.json')
        for k, v in self.DEFAULTS.items():
            setattr(self, k, v)

    def load(self):
        try:
            with open(self.path, 'r', encoding='utf-8') as f:
                data = json.load(f)
            for old_key, new_key in self._LEGACY_MAP.items():
                if old_key in data:
                    setattr(self, new_key, data[old_key])
                elif new_key in data:
                    setattr(self, new_key, data[new_key])
        except FileNotFoundError:
            pass

    def save(self):
        data = {k: getattr(self, k) for k in self.DEFAULTS}
        try:
            with open(self.path, 'w', encoding='utf-8') as f:
                json.dump(data, f, indent=4, ensure_ascii=False)
        except Exception as e:
            print(f"설정 저장 실패: {e}")


class RestartSignal(Exception):
    pass


class SwordMacro:
    def __init__(self):
        self.cfg = Config()
        self.cfg.load()
        self.overlay = create_overlay()

        # 런타임 상태
        self.paused = False
        self.restart = False

        # 세션 통계
        self.stats = {
            'trash': 0, 'hidden': 0, 'destroy': 0,
            'enhance_ok': 0, 'enhance_hold': 0,
            'gold_first': None, 'gold_last': None,
            'started_at': None,
            'cycles': 0, 'cycle_gold_sum': 0, 'cycle_sec_sum': 0.0,
        }

        # 사이클 추적 (파밍→강화→판매 = 1사이클)
        self._cycle_id = 0
        self._cycle_start = None
        self._cycle_gold_start = None

        # 로깅
        self._log_handle = None
        self._panel_lines = []
        self._data_writer = None
        self._data_file = None

        # 핫키 등록
        self._setup_hotkeys()

        # 종료 시 정리
        atexit.register(self._cleanup)
        signal.signal(signal.SIGTERM, lambda *_: (self._cleanup(), exit(0)))

    # ─── 핫키 ────────────────────────────────────
    def _setup_hotkeys(self):
        def on_key(key):
            try:
                if key == pynput_keyboard.Key.f8:
                    self.paused = not self.paused
                    tag = "[II] 일시정지" if self.paused else "[>] 재개"
                    print(f"\n{tag}")
                elif key == pynput_keyboard.Key.f9:
                    if not self.restart:
                        print("\n[F9] 재시작 요청!")
                        self.restart = True
            except Exception:
                pass

        listener = pynput_keyboard.Listener(on_press=on_key)
        listener.daemon = True
        listener.start()

    # ─── 상태 체크 ───────────────────────────────
    def _check(self):
        if self.restart:
            raise RestartSignal()
        while self.paused:
            time.sleep(0.1)
            if self.restart:
                raise RestartSignal()

    # ─── 로깅 ────────────────────────────────────
    def _open_log(self):
        if self._log_handle is not None:
            return
        log_path = os.path.join(_SCRIPT_DIR, 'sword_macro.log')
        if os.path.exists(log_path) and os.path.getsize(log_path) > LOG_MAX_BYTES:
            bak = log_path + '.bak'
            if os.path.exists(bak):
                os.remove(bak)
            os.rename(log_path, bak)
        self._log_handle = open(log_path, 'a', encoding='utf-8')
        self._log_handle.write(f"\n{'=' * 60}\n")
        self._log_handle.write(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] 세션 시작\n")
        self._log_handle.write(f"{'=' * 60}\n")
        self._log_handle.flush()
        self.stats['started_at'] = time.time()

    def _open_data_log(self):
        """분석용 CSV 데이터 로그 초기화"""
        if self._data_writer is not None:
            return
        data_path = os.path.join(_SCRIPT_DIR, 'sword_data.csv')
        is_new = not os.path.exists(data_path) or os.path.getsize(data_path) == 0
        self._data_file = open(data_path, 'a', newline='', encoding='utf-8')
        self._data_writer = csv.writer(self._data_file)
        if is_new:
            self._data_writer.writerow([
                'timestamp', 'event', 'level', 'result', 'gold', 'item', 'mode',
                'cycle_id', 'cycle_sec', 'gold_earned'
            ])
            self._data_file.flush()

    def _record(self, event, level=None, result=None, gold=None, item=None, mode=None,
                cycle_sec=None, gold_earned=None):
        """구조화 데이터 1행 기록
        event: enhance / sell / farm / destroy / goal / cycle_end
        level: 현재 강화 레벨
        result: success / hold / destroy / trash / hidden
        gold: 기록 시점 보유 골드
        item: 아이템명 (파밍 시)
        mode: target / hidden / money
        cycle_sec: 사이클 소요 시간 (cycle_end 시)
        gold_earned: 사이클 벌이 (cycle_end 시)
        """
        try:
            self._open_data_log()
            self._data_writer.writerow([
                time.strftime('%Y-%m-%d %H:%M:%S'),
                event,
                level if level is not None else '',
                result or '',
                gold if gold is not None else (self.stats['gold_last'] or ''),
                item or '',
                mode or '',
                self._cycle_id if self._cycle_id > 0 else '',
                f"{cycle_sec:.1f}" if cycle_sec is not None else '',
                gold_earned if gold_earned is not None else '',
            ])
            self._data_file.flush()
        except Exception:
            pass

    def _log(self, msg):
        line = f"[{time.strftime('%H:%M:%S')}] {msg}"
        print(line)

        # 오버레이 패널 업데이트
        self._panel_lines.append(line)
        if len(self._panel_lines) > 25:
            self._panel_lines.pop(0)
        try:
            with open(STATUS_FILE, 'w', encoding='utf-8') as f:
                f.write('\n'.join(self._panel_lines))
        except Exception:
            pass

        # 파일 로그
        try:
            self._open_log()
            self._log_handle.write(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] {msg}\n")
            self._log_handle.flush()
        except Exception:
            pass

    def _log_summary(self):
        elapsed = time.time() - self.stats['started_at'] if self.stats['started_at'] else 0
        m, s = int(elapsed // 60), int(elapsed % 60)
        g0 = self.stats['gold_first'] or 0
        g1 = self.stats['gold_last'] or 0
        diff = g1 - g0
        sign = '+' if diff >= 0 else ''
        gph = int(diff / elapsed * 3600) if elapsed > 0 else 0
        gph_sign = '+' if gph >= 0 else ''

        cyc = self.stats['cycles']
        avg_cyc = self.stats['cycle_sec_sum'] / cyc if cyc else 0
        avg_earn = int(self.stats['cycle_gold_sum'] / cyc) if cyc else 0

        text = (
            f"\n{'─' * 50}\n"
            f"  📊 세션 통계 ({m}분 {s}초)\n"
            f"{'─' * 50}\n"
            f"  트래시 판매: {self.stats['trash']}회\n"
            f"  히든 발견:   {self.stats['hidden']}회\n"
            f"  강화 성공:   {self.stats['enhance_ok']}회\n"
            f"  강화 유지:   {self.stats['enhance_hold']}회\n"
            f"  강화 파괴:   {self.stats['destroy']}회\n"
            f"  골드 변화:   {g0:,}G → {g1:,}G ({sign}{diff:,}G)\n"
            f"{'─' * 50}\n"
            f"  💰 시간당 골드: {gph_sign}{gph:,}G/h\n"
            f"  🔄 완료 사이클: {cyc}회 (평균 {avg_cyc:.0f}초, {avg_earn:+,}G/사이클)\n"
            f"{'─' * 50}"
        )
        print(text)
        if self._log_handle:
            self._log_handle.write(text + '\n')
            self._log_handle.flush()

    # ─── OCR ─────────────────────────────────────
    def _capture_ocr(self, region):
        x, y, w, h = region
        img = platform_capture.capture_region(x, y, w, h)
        if img is None:
            return ""
        return platform_ocr.recognize_text(img)

    def _read_chat(self, cx, start_y):
        self._check()
        cap_x = max(cx - CAPTURE_W // 2, 0)
        raw = self._capture_ocr((cap_x, start_y, CAPTURE_W, CAPTURE_H))

        # OCR 원문을 로그에 기록
        try:
            self._open_log()
            self._log_handle.write(f"--- OCR RAW [{time.strftime('%H:%M:%S')}] ---\n")
            self._log_handle.write(raw.strip() or "(empty)")
            self._log_handle.write("\n--- OCR END ---\n")
            self._log_handle.flush()
        except Exception:
            pass

        if not raw.strip():
            self._log("📷 OCR: (텍스트 없음)")
            return ""

        lines = raw.strip().split('\n')
        recent = lines[-3:] if len(lines) > 3 else lines
        self._log(f"📷 OCR ({len(lines)}줄): " + " | ".join(recent))
        return raw

    # ─── 입력 ────────────────────────────────────
    def _send(self, cmd, x, y):
        self._check()
        pyautogui.click(x, y)
        time.sleep(self.cfg.input_delay)
        pyautogui.hotkey(_MOD_KEY, 'a')
        time.sleep(self.cfg.input_delay)
        pyautogui.press('backspace')
        time.sleep(self.cfg.input_delay)

        pyperclip.copy("")
        for _ in range(5):
            self._check()
            pyperclip.copy(cmd)
            time.sleep(self.cfg.clipboard_delay)
            clip = pyperclip.paste().strip()
            if len(clip) > 20:
                pyperclip.copy("")
                continue
            if clip == cmd:
                time.sleep(self.cfg.input_delay)
                pyautogui.hotkey(_MOD_KEY, 'v')
                time.sleep(self.cfg.input_delay)
                pyautogui.press('enter')
                time.sleep(self.cfg.input_delay)
                pyautogui.press('enter')
                return
        self._log("입력 실패 - 턴 스킵")

    # ─── 골드 체크 ───────────────────────────────
    def _check_gold(self, log):
        matches = RE_GOLD.findall(log)
        if matches:
            gold = int(matches[-1].replace(',', ''))
            if self.stats['gold_first'] is None:
                self.stats['gold_first'] = gold
            self.stats['gold_last'] = gold
            if self.cfg.min_gold > 0 and gold <= self.cfg.min_gold:
                self._log(f"골드 제한 도달: {gold:,}G")
                return False
        return True

    # ─── 골드 기반 목표 계산 ────────────────────
    def _gold_target(self):
        return GOLD_MINE_TARGET

    # ─── 사이클 추적 ─────────────────────────────
    def _begin_cycle(self):
        """새 사이클(파밍→강화→판매) 시작"""
        self._cycle_id += 1
        self._cycle_start = time.time()
        self._cycle_gold_start = self.stats['gold_last']

    def _end_cycle(self, mode=None):
        """사이클 종료: 소요시간·벌이 기록, 누적 통계 갱신"""
        if self._cycle_start is None:
            return
        elapsed = time.time() - self._cycle_start
        g_start = self._cycle_gold_start or 0
        g_end = self.stats['gold_last'] or 0
        earned = g_end - g_start

        self.stats['cycles'] += 1
        self.stats['cycle_gold_sum'] += earned
        self.stats['cycle_sec_sum'] += elapsed

        avg_sec = self.stats['cycle_sec_sum'] / self.stats['cycles']
        avg_gold = self.stats['cycle_gold_sum'] / self.stats['cycles']
        gph = int(self.stats['cycle_gold_sum'] / self.stats['cycle_sec_sum'] * 3600) if self.stats['cycle_sec_sum'] > 0 else 0

        self._record('cycle_end', gold=g_end, mode=mode,
                     cycle_sec=elapsed, gold_earned=earned)
        self._log(f"📦 사이클 #{self._cycle_id} 완료: "
                  f"{elapsed:.0f}초, {earned:+,}G | "
                  f"평균 {avg_sec:.0f}초/{avg_gold:+,.0f}G | "
                  f"시간당 {gph:,}G/h")

        self._cycle_start = None

    # ─── 목표 달성 처리 ─────────────────────────
    def _handle_goal(self, mode, auto_sell, ix, iy):
        """목표 달성 시 모드별 분기. True=계속, False=중단, None=판매후계속"""
        if mode == MODE_TARGET:
            return False
        elif mode == MODE_HIDDEN:
            if auto_sell:
                self._log("⚡ 판단: 판매 후 재파밍")
                return True
            return False
        elif mode == MODE_MONEY:
            self._log("⚡ 판단: 목표 도달 → /판매")
            self._send(CMD_SELL, ix, iy)
            time.sleep(self.cfg.fast_delay)
            self._end_cycle(mode)
            return True
        return False

    # ─── 파밍 루프 ───────────────────────────────
    def _run_farming(self, ix, iy, start_y):
        """파밍 1턴. 반환: ('farming'|'enhancing'|'undecided'|'stop', _)"""
        self._log("── 파밍: /판매 전송 ──")
        self._send(CMD_SELL, ix, iy)
        time.sleep(self.cfg.fast_delay)

        log = self._read_chat(ix, start_y)
        if not log.strip():
            self._log("⚡ 판단: 내 메시지 없음 → /강화")
            self._send(CMD_ENHANCE, ix, iy)
            time.sleep(self.cfg.fast_delay)
            return 'farming', 0

        if not self._check_gold(log):
            return 'stop', 0

        if "판매할 수 없다" in log or "가치가 없어서" in log:
            self._log("⚡ 판단: 0강 판매 불가 → /강화")
            self._send(CMD_ENHANCE, ix, iy)
            time.sleep(self.cfg.fast_delay)
            return 'farming', 0

        if "새로운 검 획득:" in log:
            item_raw = log.split("새로운 검 획득:")[-1].strip()
            item_name = ' '.join(item_raw.split('\n')[:3])

            is_trash = any(t in item_name for t in TRASH_ITEMS) or "낡은" in item_name

            if is_trash:
                self.stats['trash'] += 1
                self._record('farm', level=0, result='trash', item=item_name[:30])
                self._log(f"⚡ 판단: 트래시 ({item_name[:20]}) → /강화")
                self._send(CMD_ENHANCE, ix, iy)
                time.sleep(TRASH_DELAY)
                return 'farming', 0
            else:
                self.stats['hidden'] += 1
                self._record('farm', level=0, result='hidden', item=item_name[:30])
                self._log(f"🎉 히든 아이템! ({item_name[:30]}) → 강화 모드")
                return 'enhancing', 0

        # 판별 불가
        return 'undecided', 0

    # ─── 강화 루프 ───────────────────────────────
    def _run_enhancing(self, ix, iy, start_y, target, stop_num, mode, auto_sell, delay):
        """강화 1턴. 반환: (다음상태, 딜레이)"""
        self._log("── 강화: /강화 전송 ──")
        self._send(CMD_ENHANCE, ix, iy)
        time.sleep(delay)

        log = self._read_chat(ix, start_y)
        if not log.strip():
            self._log("⚡ 판단: 내 메시지 없음 → 재강화")
            return 'enhancing', delay

        if "골드가 부족해" in log:
            self._log("💰 골드 부족! 중단")
            return 'stop', delay

        if not self._check_gold(log):
            return 'stop', delay

        # 강화 결과 파싱 + 레벨 추출
        level_matches = RE_LEVEL.findall(log)
        cur_level = int(level_matches[-1]) if level_matches else None

        if "강화 성공" in log:
            self.stats['enhance_ok'] += 1
            self._record('enhance', level=cur_level, result='success', mode=mode)
        elif "강화 유지" in log:
            self.stats['enhance_hold'] += 1
            self._record('enhance', level=cur_level, result='hold', mode=mode)

        if mode in (MODE_HIDDEN, MODE_MONEY) and "강화 파괴" in log:
            self.stats['destroy'] += 1
            self._record('enhance', level=0, result='destroy', mode=mode)
            self._log("💀 검 파괴됨 → 파밍 복귀")
            self._end_cycle(mode)
            return 'farming', self.cfg.fast_delay

        # 목표 문자열 직접 매칭
        if target in log:
            self._log(f"🏆 목표 달성! {target}")
            self._record('goal', level=cur_level, result='reached', mode=mode)
            result = self._handle_goal(mode, auto_sell, ix, iy)
            if result:
                self._record('sell', level=cur_level, mode=mode)
                return 'farming', self.cfg.fast_delay
            return 'stop', delay

        # 레벨 숫자 파싱
        if cur_level is not None:
            self._log(f"⚔️  현재 강화: +{cur_level} (목표: +{stop_num})")

            if cur_level >= stop_num:
                self._log(f"🏆 목표 도달! (+{cur_level})")
                self._record('goal', level=cur_level, result='reached', mode=mode)
                result = self._handle_goal(mode, auto_sell, ix, iy)
                if result:
                    self._record('sell', level=cur_level, mode=mode)
                    return 'farming', self.cfg.fast_delay
                return 'stop', delay

            if cur_level >= self.cfg.slow_start_level:
                self._log(f"🐢 고강 감속: {self.cfg.slow_delay}초")
                return 'enhancing', self.cfg.slow_delay
            if cur_level <= BOOST_LEVEL:
                return 'enhancing', BOOST_DELAY
            return 'enhancing', self.cfg.fast_delay

        return 'enhancing', delay

    # ─── 좌표 설정 ───────────────────────────────
    def _setup_coords(self):
        """좌표 설정. 반환: (input_x, input_y, log_start_y)"""
        if self.cfg.use_fixed_pos:
            if self.cfg.fixed_x and self.cfg.fixed_y:
                self._log(f"저장된 좌표 사용: {self.cfg.fixed_x}, {self.cfg.fixed_y}")
                self.overlay.show_at(self.cfg.fixed_x, self.cfg.fixed_y)
                time.sleep(1)
                return self.cfg.fixed_x, self.cfg.fixed_y, self.cfg.fixed_start_y

            # 좌표 마법사
            print("\n[좌표 설정 마법사]")
            print("1. 카카오톡 메시지 입력 칸에 마우스를 올리세요 (3초)")
            time.sleep(3)
            ix, iy = pyautogui.position()
            print("2. 채팅 로그 시작점(위쪽)에 마우스를 올리세요 (3초)")
            time.sleep(3)
            _, sy = pyautogui.position()
            self.cfg.fixed_x, self.cfg.fixed_y, self.cfg.fixed_start_y = ix, iy, sy
            self.cfg.use_fixed_pos = True
            self.cfg.save()
            self.overlay.show_at(ix, iy)
            return ix, iy, sy

        # 자동 설정
        print("\n" + "=" * 50)
        print("[좌표 설정]")
        print("=" * 50)
        print()
        print("카카오톡 메시지 입력 칸에 마우스를 올리세요!")
        print("(3초 후 자동으로 좌표를 잡고 오버레이를 표시합니다)")
        _countdown(3)

        anchor_x, anchor_y = pyautogui.position()
        self.overlay.show_at(anchor_x, anchor_y)

        print(f"\n   -> 기준점: ({anchor_x}, {anchor_y})")
        print(f"   -> OCR 캡처: {CAPTURE_W}x{CAPTURE_H}")
        print()
        print("=" * 50)
        print("  초록 테두리 = OCR 캡처 영역 (채팅 로그)")
        print("  빨간 테두리 = 입력창 영역")
        print("  카톡 창을 테두리에 맞추세요!")
        print("=" * 50)
        print("(5초 후 매크로가 시작됩니다)")
        _countdown(5)

        ix = anchor_x
        iy = anchor_y
        sy = (anchor_y - INPUT_BOX_H // 2) - CAPTURE_H
        print(f"   -> 입력 클릭: ({ix}, {iy})")
        print(f"   -> OCR 시작: y={sy}")
        return ix, iy, sy

    # ─── 설정 메뉴 ───────────────────────────────
    def _settings_menu(self):
        options = [
            ('감속 시작 레벨', 'slow_start_level', int, "이 레벨부터 강화 딜레이 증가"),
            ('일반 속도', 'fast_delay', float, "중간 레벨(+5~+8) 강화 대기"),
            ('고강 속도', 'slow_delay', float, "고레벨(+9~) 강화 대기"),
            ('최소 골드', 'min_gold', int, "골드가 이 값 이하면 자동 중단"),
            ('클립보드 안전 시간', 'clipboard_delay', float, "렉 걸리면 올리세요"),
            ('입력 딜레이', 'input_delay', float, "명령어 씹히면 올리세요"),
        ]

        while True:
            print("\n[옵션 설정]")
            for i, opt in enumerate(options, 1):
                val = getattr(self.cfg, opt[1])
                unit = '강' if opt[1] == 'slow_start_level' else ('G' if opt[1] == 'min_gold' else '초')
                hint = f" *{opt[3]}" if len(opt) > 3 else ""
                print(f"{i}. {opt[0]} ({val}{unit}){hint}")
            print(f"7. 좌표 고정 ({'ON' if self.cfg.use_fixed_pos else 'OFF'})")
            print("8. 좌표 직접 입력")
            print("9. 뒤로 가기")

            sel = input("변경할 번호: ").strip()

            if sel in [str(i) for i in range(1, len(options) + 1)]:
                idx = int(sel) - 1
                opt = options[idx]
                try:
                    val = opt[2](input("값: "))
                    setattr(self.cfg, opt[1], val)
                    self.cfg.save()
                except Exception:
                    pass
            elif sel == '7':
                self.cfg.use_fixed_pos = not self.cfg.use_fixed_pos
                if not self.cfg.use_fixed_pos:
                    self.cfg.fixed_x = None
                self.cfg.save()
            elif sel == '8':
                try:
                    self.cfg.fixed_x = int(input("X: "))
                    self.cfg.fixed_y = int(input("Y: "))
                    self.cfg.fixed_start_y = int(input("Start Y: "))
                    self.cfg.use_fixed_pos = True
                    self.cfg.save()
                except Exception:
                    print("숫자만 입력하세요")
            elif sel in ('9', ''):
                break

    # ─── 정리 ────────────────────────────────────
    def _cleanup(self):
        self.overlay.hide()
        if self._data_file:
            try:
                self._data_file.close()
            except Exception:
                pass

    # ─── 메인 ────────────────────────────────────
    def run(self):
        # OCR 엔진 초기화
        platform_ocr.init_ocr()

        print("\n" + "=" * 50)
        print("  sword-macro-ai — 검키우기 자동화 + AI 데이터 분석")
        print("=" * 50)

        if sys.platform == 'darwin':
            print()
            print("  [접근성 권한 필수]")
            print("  시스템 설정 > 개인정보 보호 및 보안 > 접근성")
            print("  에서 터미널(또는 사용 중인 앱)을 허용하세요.")
        print()
        print("  [조작 키]")
        print("  F8  일시정지 / 재개")
        print("  F9  재시작 (메뉴로 복귀)")
        print("  마우스 좌상단 모서리 → 비상 정지")
        print("=" * 50 + "\n")

        while True:
            self.restart = False
            self.paused = False

            # 메뉴
            while True:
                pos_str = "OFF"
                if self.cfg.use_fixed_pos:
                    pos_str = f"ON ({self.cfg.fixed_x},{self.cfg.fixed_y})" if self.cfg.fixed_x else "ON (마법사)"

                print("\n" * 3)
                print("=== 카카오톡 검키우기 ===")
                print(f"   [속도] {self.cfg.slow_start_level}강부터 감속 | "
                      f"일반 {self.cfg.fast_delay}초 | 고강 {self.cfg.slow_delay}초")
                print(f"   [자산] 최소 골드: {self.cfg.min_gold:,}G")
                print(f"   [좌표] 고정 모드: {pos_str}")
                print("─" * 39)
                print("1. 강화 목표 달성  — 설정한 레벨까지 자동 강화")
                print("2. 히든 검 뽑기    — 히든 아이템까지 자동 파밍+강화")
                print("3. 골드 채굴       — 파밍→강화(+10)→판매 무한 순환")
                print("4. 옵션 설정")
                print("=" * 39)

                try:
                    sel = input("선택 (1~4): ").strip()
                except EOFError:
                    return

                if sel == '4':
                    self._settings_menu()
                elif sel in (MODE_TARGET, MODE_HIDDEN, MODE_MONEY):
                    mode = sel
                    break
                else:
                    print("잘못된 입력")

            # 목표 레벨
            auto_sell = False
            if mode == MODE_MONEY:
                stop_num = GOLD_MINE_TARGET
                target = f"[+{stop_num}]"
                auto_sell = True
                print(f"\n[골드 채굴 모드] 히든 파밍 → +{stop_num} 강화 → 판매 순환")
            else:
                while True:
                    try:
                        stop_num = int(input("\n몇 강까지?: "))
                        target = f"[+{stop_num}]"
                        break
                    except Exception:
                        pass
                if mode == MODE_HIDDEN:
                    print("1. 목표 달성 시 멈춤")
                    print("2. 판매 후 다시 뽑기(무한)")
                    if input("선택: ") == '2':
                        auto_sell = True

            # 좌표 설정
            ix, iy, start_y = self._setup_coords()

            self._log(f"OCR 캡처 영역: {CAPTURE_W}x{CAPTURE_H} [초록 테두리]")
            self._log("매크로 시작 (F8:일시정지, F9:재시작)")

            # 게임 루프
            # 골드 채굴/히든: 파밍부터 시작 (히든 뽑기) / 나머지: 강화부터
            state = 'farming' if mode in (MODE_HIDDEN, MODE_MONEY) else 'enhancing'
            delay = self.cfg.fast_delay
            undecided = 0

            try:
                while True:
                    self._check()

                    if state == 'farming':
                        next_state, _ = self._run_farming(ix, iy, start_y)

                        if next_state == 'undecided':
                            undecided += 1
                            if undecided >= 3:
                                wait = min(undecided, 8)
                                self._log(f"⚡ 판단: 판별 불가 ({undecided}연속) → {wait}초 대기")
                                time.sleep(wait)
                            else:
                                self._log("⚡ 판단: 로그 판별 불가 → /강화")
                                self._send(CMD_ENHANCE, ix, iy)
                                time.sleep(self.cfg.fast_delay)
                        elif next_state == 'stop':
                            break
                        else:
                            undecided = 0
                            if next_state == 'enhancing' and state == 'farming':
                                self._begin_cycle()
                            state = next_state
                            delay = BOOST_DELAY  # 히든 첫 강화는 부스트

                    elif state == 'enhancing':
                        state, delay = self._run_enhancing(
                            ix, iy, start_y, target, stop_num, mode, auto_sell, delay)
                        if state == 'stop':
                            break

            except RestartSignal:
                self.overlay.hide()
                self._log("재시작 처리 중...")
                continue
            except KeyboardInterrupt:
                self.overlay.hide()
                self._log_summary()
                print("\n사용자 종료")
                break
            except Exception as e:
                self.overlay.hide()
                self._log_summary()
                print(f"\n에러 발생: {e}")
                break

            self._log_summary()
            self.overlay.hide()
            if input("R 입력 시 재시작: ").lower() != 'r':
                break


# ─── 유틸리티 ────────────────────────────────────
def _countdown(sec):
    for i in range(sec, 0, -1):
        print(f"  {i}...", flush=True)
        time.sleep(1)


# ─── 실행 ────────────────────────────────────────
if __name__ == '__main__':
    SwordMacro().run()
