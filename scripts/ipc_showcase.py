#!/usr/bin/env python3
"""Phalune IPC Widget Extension Showcase.

Demonstrates bi-directional communication with Phalune over Unix domain sockets:
streaming state updates (text, icon, progress, CSS classes, declarative popovers)
and handling real-time user interaction events from the bar.
"""

import argparse
import json
import os
import select
import signal
import socket
import sys
import threading
import time
from typing import Any, Callable, Dict, List, Optional

DEFAULT_WORK_DURATION = 25 * 60
DEFAULT_BREAK_DURATION = 5 * 60


class PhaluneIPCClient:
    """Manages a bi-directional socket connection to the Phalune desktop shell."""

    def __init__(self, socket_path: Optional[str] = None):
        self.socket_path = socket_path or self.default_socket_path()
        self.sock: Optional[socket.socket] = None
        self._running = False
        self._recv_buffer = ""
        self._listener_thread: Optional[threading.Thread] = None

    @staticmethod
    def default_socket_path() -> str:
        runtime_dir = os.environ.get("XDG_RUNTIME_DIR")
        if runtime_dir:
            return os.path.join(runtime_dir, "phalune.sock")
        return f"/tmp/phalune-{os.getuid()}.sock"

    def connect(self) -> None:
        if not os.path.exists(self.socket_path):
            raise FileNotFoundError(
                f"Phalune socket not found at '{self.socket_path}'. Is Phalune running?"
            )
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(self.socket_path)
        self._running = True

    def send_line(self, data: Dict[str, Any]) -> bool:
        if not self.sock or not self._running:
            return False
        try:
            payload = json.dumps(data).encode("utf-8") + b"\n"
            self.sock.sendall(payload)
            return True
        except (BrokenPipeError, ConnectionResetError, OSError):
            self._running = False
            return False

    def read_line(self, timeout: Optional[float] = 5.0) -> Optional[Dict[str, Any]]:
        """Reads a single newline-delimited JSON line. Useful for synchronous handshakes."""
        start = time.time()
        while self._running and self.sock:
            if "\n" in self._recv_buffer:
                line, self._recv_buffer = self._recv_buffer.split("\n", 1)
                line = line.strip()
                if line:
                    try:
                        return json.loads(line)
                    except json.JSONDecodeError:
                        return None
            remaining = None if timeout is None else max(0.0, timeout - (time.time() - start))
            if timeout is not None and remaining <= 0:
                return None
            r, _, _ = select.select([self.sock], [], [], remaining if remaining is not None else 0.5)
            if not r:
                if timeout is not None:
                    return None
                continue
            try:
                chunk = self.sock.recv(4096)
                if not chunk:
                    self._running = False
                    return None
                self._recv_buffer += chunk.decode("utf-8", errors="replace")
            except (BrokenPipeError, ConnectionResetError, OSError):
                self._running = False
                return None
        return None

    def claim_widget(self, widget_id: str, clear_on_disconnect: bool = True) -> Dict[str, Any]:
        args = {"id": widget_id}
        if clear_on_disconnect:
            args["clear_on_disconnect"] = "true"
        req = {"action": "widget-watch", "args": args}
        if not self.send_line(req):
            raise ConnectionError("Failed to send widget-watch request")
        handshake = self.read_line(timeout=5.0)
        if not handshake:
            raise ConnectionError("Server closed connection or timed out during handshake")
        if not handshake.get("ok"):
            error_msg = handshake.get("error", "Unknown error")
            raise RuntimeError(f"Failed to claim widget '{widget_id}': {error_msg}")
        return handshake

    def clear_widget(self, widget_id: str) -> bool:
        """Clears and hides the widget from the bar."""
        return self.send_line({"id": widget_id, "text": "", "icon": ""})

    def start_listener(self, on_event: Callable[[Dict[str, Any]], None]) -> None:
        def _listen_loop():
            while self._running and self.sock:
                try:
                    if "\n" in self._recv_buffer:
                        line, self._recv_buffer = self._recv_buffer.split("\n", 1)
                        line = line.strip()
                        if line:
                            try:
                                event = json.loads(line)
                                on_event(event)
                            except json.JSONDecodeError:
                                pass
                        continue

                    r, _, _ = select.select([self.sock], [], [], 0.2)
                    if not r:
                        continue
                    chunk = self.sock.recv(4096)
                    if not chunk:
                        break
                    self._recv_buffer += chunk.decode("utf-8", errors="replace")
                except Exception as exc:
                    if self._running:
                        print(f"[\033[31mERROR\033[0m] Read error: {exc}", file=sys.stderr)
                    break
            self._running = False

        self._listener_thread = threading.Thread(target=_listen_loop, daemon=True)
        self._listener_thread.start()

    def send_notification(self, summary: str, body: str = "", icon: str = "") -> None:
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
                s.connect(self.socket_path)
                req = {
                    "action": "notify",
                    "args": {"summary": summary, "body": body, "icon": icon},
                }
                s.sendall(json.dumps(req).encode("utf-8") + b"\n")
        except Exception as exc:
            print(f"[\033[33mWARN\033[0m] Failed to send notification: {exc}", file=sys.stderr)

    def close(self) -> None:
        self._running = False
        if self.sock:
            try:
                self.sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            try:
                self.sock.close()
            except OSError:
                pass
            self.sock = None


class PomodoroShowcase:
    """Interactive Pomodoro timer with buttons, countdown, and progress."""

    def __init__(self, client: PhaluneIPCClient, widget_id: str):
        self.client = client
        self.widget_id = widget_id
        self.work_duration = DEFAULT_WORK_DURATION
        self.break_duration = DEFAULT_BREAK_DURATION
        self.time_left = self.work_duration
        self.is_work = True
        self.is_running = False
        self.completed_sessions = 0
        self.lock = threading.Lock()

    def handle_event(self, event: Dict[str, Any]) -> None:
        event_name = event.get("event")
        action = event.get("action")

        print(f"[\033[36mEVENT\033[0m] Interaction: {json.dumps(event)}")

        with self.lock:
            if event_name == "popover_action":
                if action == "btn_toggle":
                    self.is_running = not self.is_running
                elif action == "btn_reset":
                    self.time_left = self.work_duration if self.is_work else self.break_duration
                    self.is_running = False
                elif action == "btn_switch_mode":
                    self.is_work = not self.is_work
                    self.time_left = self.work_duration if self.is_work else self.break_duration
                    self.is_running = False
                elif action == "btn_skip":
                    self._on_session_finished()
                self._push_state_locked()

            elif event_name == "widget_clicked":
                button = event.get("button", 1)
                if button == 1:
                    self.is_running = not self.is_running
                elif button == 3:
                    self.time_left = self.work_duration if self.is_work else self.break_duration
                self._push_state_locked()

    def _on_session_finished(self) -> None:
        if self.is_work:
            self.completed_sessions += 1
            self.client.send_notification(
                "🍅 Pomodoro Complete!",
                f"Session #{self.completed_sessions} finished. Take a 5-minute break.",
                icon="alarm-symbolic",
            )
            self.is_work = False
            self.time_left = self.break_duration
        else:
            self.client.send_notification(
                "☕ Break Over!",
                "Ready to focus? Starting a new 25-minute work sprint.",
                icon="appointment-soon-symbolic",
            )
            self.is_work = True
            self.time_left = self.work_duration
        self.is_running = False

    def build_popover_spec(self, total: int, fraction: float) -> Dict[str, Any]:
        mode_title = "Focus Session" if self.is_work else "Rest Break"
        status_label = "Running" if self.is_running else "Paused"

        return {
            "title": f"🍅 Pomodoro Timer ({mode_title})",
            "rows": [
                {
                    "type": "progress",
                    "label": f"{mode_title}: {self.time_left // 60}m {self.time_left % 60}s remaining ({status_label})",
                    "value": round(fraction, 3),
                },
                {
                    "type": "button_row",
                    "buttons": [
                        {
                            "id": "btn_toggle",
                            "label": "⏸ Pause" if self.is_running else "▶ Start",
                            "class": "suggested-action" if not self.is_running else "",
                        },
                        {"id": "btn_skip", "label": "⏭ Skip"},
                        {"id": "btn_reset", "label": "🔄 Reset"},
                        {
                            "id": "btn_switch_mode",
                            "label": "☕ Switch to Break" if self.is_work else "💼 Switch to Work",
                        },
                    ],
                },
                {
                    "type": "list",
                    "items": [
                        f"Completed sessions: {self.completed_sessions}",
                        f"Target today: 8 sessions ({self.completed_sessions}/8)",
                        "Left-click toggles timer; click opens full controls",
                    ],
                },
                {
                    "type": "label",
                    "label": "<small><i>Phalune IPC Extension</i></small>",
                },
            ],
        }

    def _push_state_locked(self) -> None:
        total = self.work_duration if self.is_work else self.break_duration
        elapsed = total - self.time_left
        fraction = max(0.0, min(1.0, elapsed / total if total > 0 else 0.0))
        percentage = round(fraction * 100, 1)

        minutes, seconds = divmod(self.time_left, 60)
        time_str = f"{minutes:02d}:{seconds:02d}"

        if self.is_running:
            text = f"<b>{time_str}</b>"
            css_class = "running"
        else:
            text = f"<span foreground='#9ece6a'><b>{time_str}</b></span> (paused)"
            css_class = "paused"

        icon_name = "alarm-symbolic" if self.is_work else "weather-clear-symbolic"
        tooltip = f"Pomodoro: {time_str} left ({'Work' if self.is_work else 'Break'})"

        state = {
            "id": self.widget_id,
            "text": text,
            "tooltip": tooltip,
            "icon": icon_name,
            "class": css_class,
            "percentage": percentage,
            "popover": self.build_popover_spec(total, fraction),
        }

        self.client.send_line(state)
        print(f"[\033[32mPUSH\033[0m] {time_str} ({percentage}%) -> bar pill updated")

    def run(self) -> None:
        with self.lock:
            self._push_state_locked()

        while self.client._running:
            time.sleep(1)
            with self.lock:
                if not self.client._running:
                    break
                if self.is_running:
                    self.time_left -= 1
                    if self.time_left <= 0:
                        self._on_session_finished()
                    self._push_state_locked()


class SystemMonitorShowcase:
    """Real-time system telemetry monitor with warning thresholds."""

    def __init__(self, client: PhaluneIPCClient, widget_id: str):
        self.client = client
        self.widget_id = widget_id
        self.view_mode = "cpu"
        self._prev_idle = 0.0
        self._prev_total = 0.0
        self.lock = threading.Lock()

    def handle_event(self, event: Dict[str, Any]) -> None:
        action = event.get("action")
        print(f"[\033[36mEVENT\033[0m] Interaction: {json.dumps(event)}")
        with self.lock:
            if action == "btn_toggle_metric" or event.get("event") == "widget_clicked":
                self.view_mode = "mem" if self.view_mode == "cpu" else "cpu"
                self._update_and_push_locked()
            elif action == "btn_notify":
                self.client.send_notification("⚡ System Health OK", "All hardware metrics within nominal ranges.")

    def _read_cpu_percent(self) -> float:
        try:
            with open("/proc/stat", "r", encoding="utf-8") as f:
                fields = [float(column) for column in f.readline().strip().split()[1:8]]
            idle, total = fields[3], sum(fields)
            idle_delta, total_delta = idle - self._prev_idle, total - self._prev_total
            self._prev_idle, self._prev_total = idle, total
            if total_delta > 0:
                return round(100.0 * (1.0 - idle_delta / total_delta), 1)
        except Exception:
            pass
        return 24.5

    def _read_mem_percent(self) -> float:
        try:
            mem = {}
            with open("/proc/meminfo", "r", encoding="utf-8") as f:
                for line in f:
                    parts = line.split(":")
                    if len(parts) == 2:
                        mem[parts[0].strip()] = int(parts[1].split()[0])
            total = mem.get("MemTotal", 1)
            avail = mem.get("MemAvailable", mem.get("MemFree", 0))
            used = total - avail
            return round(100.0 * used / total, 1)
        except Exception:
            pass
        return 42.0

    def _update_and_push_locked(self) -> None:
        cpu_pct = self._read_cpu_percent()
        mem_pct = self._read_mem_percent()

        if self.view_mode == "cpu":
            current_pct = cpu_pct
            color = "#f7768e" if cpu_pct > 80 else ("#e0af68" if cpu_pct > 60 else "#9ece6a")
            text = f"CPU <span foreground='{color}'><b>{cpu_pct:.0f}%</b></span>"
            icon = "utilities-system-monitor-symbolic"
        else:
            current_pct = mem_pct
            color = "#f7768e" if mem_pct > 85 else ("#e0af68" if mem_pct > 70 else "#7aa2f7")
            text = f"RAM <span foreground='{color}'><b>{mem_pct:.0f}%</b></span>"
            icon = "drive-harddisk-symbolic"

        popover = {
            "title": "🖥️ System Resource Monitor",
            "rows": [
                {
                    "type": "progress",
                    "label": f"Processor Load: {cpu_pct}%",
                    "value": round(cpu_pct / 100.0, 3),
                },
                {
                    "type": "progress",
                    "label": f"Memory Usage: {mem_pct}%",
                    "value": round(mem_pct / 100.0, 3),
                },
                {
                    "type": "button_row",
                    "buttons": [
                        {
                            "id": "btn_toggle_metric",
                            "label": "⇄ Switch Metric (CPU / RAM)",
                            "class": "suggested-action",
                        },
                        {"id": "btn_notify", "label": "🔔 Test Toast"},
                    ],
                },
                {
                    "type": "list",
                    "items": [
                        f"Active display: {self.view_mode.upper()}",
                        "Updates every 2 seconds via Phalune IPC",
                    ],
                },
            ],
        }

        state = {
            "id": self.widget_id,
            "text": text,
            "tooltip": f"CPU: {cpu_pct}% | Memory: {mem_pct}% (Click to open stats)",
            "icon": icon,
            "percentage": current_pct,
            "class": "warning" if current_pct > 75 else "normal",
            "popover": popover,
        }
        self.client.send_line(state)
        print(f"[\033[32mPUSH\033[0m] {self.view_mode.upper()} {current_pct}%")

    def run(self) -> None:
        self._read_cpu_percent()
        time.sleep(0.2)
        while self.client._running:
            with self.lock:
                if not self.client._running:
                    break
                self._update_and_push_locked()
            time.sleep(2)


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Phalune IPC Widget Extension Showcase",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 scripts/ipc_showcase.py --id demo
  python3 scripts/ipc_showcase.py --demo sysmon --id sys
  python3 scripts/ipc_showcase.py --id demo --keep-on-exit
        """,
    )
    parser.add_argument(
        "--id",
        default="demo",
        help="Widget ID configured in Phalune bar (e.g. 'demo' for 'ipc:demo')",
    )
    parser.add_argument(
        "--demo",
        choices=["pomodoro", "sysmon"],
        default="pomodoro",
        help="Showcase demo mode to run (default: pomodoro)",
    )
    parser.add_argument(
        "--socket",
        default=None,
        help="Custom path to phalune.sock (defaults to $XDG_RUNTIME_DIR/phalune.sock)",
    )
    parser.add_argument(
        "--keep-on-exit",
        action="store_true",
        help="Keep the widget on the bar when this script exits (default: clear and hide on exit)",
    )

    args = parser.parse_args()

    print("\033[1;35m════════════════════════════════════════════════════════════════════\033[0m")
    print(f"\033[1;37m  Phalune IPC Extension Showcase\033[0m (\033[36m{args.demo.upper()}\033[0m)")
    print(f"  Widget Slot: \033[32mipc:{args.id}\033[0m")
    print("\033[1;35m════════════════════════════════════════════════════════════════════\033[0m")

    client = PhaluneIPCClient(args.socket)

    try:
        print(f"[*] Connecting to Phalune at \033[34m{client.socket_path}\033[0m...")
        client.connect()
        handshake = client.claim_widget(args.id, clear_on_disconnect=not args.keep_on_exit)
        print(f"[+] Successfully claimed widget '\033[32m{args.id}\033[0m'!")
        if msg := handshake.get("message"):
            print(f"    Server handshake: {msg}")
    except FileNotFoundError as exc:
        print(f"\n[\033[31mERROR\033[0m] {exc}", file=sys.stderr)
        print("Make sure Phalune is currently running (`phalune`).", file=sys.stderr)
        sys.exit(1)
    except Exception as exc:
        print(f"\n[\033[31mERROR\033[0m] Connection failed: {exc}", file=sys.stderr)
        sys.exit(1)

    app = PomodoroShowcase(client, args.id) if args.demo == "pomodoro" else SystemMonitorShowcase(client, args.id)
    client.start_listener(app.handle_event)

    _cleaned_up = False

    def _cleanup():
        nonlocal _cleaned_up
        if _cleaned_up:
            return
        _cleaned_up = True
        print("\n[*] Exiting showcase...")
        if not args.keep_on_exit:
            print(f"[*] Clearing widget '{args.id}' from bar...")
            client.clear_widget(args.id)
        client.close()

    def _sig_handler(signum, frame):
        _cleanup()
        sys.exit(0)

    signal.signal(signal.SIGINT, _sig_handler)
    signal.signal(signal.SIGTERM, _sig_handler)

    print("\n[*] \033[1;32mShowcase active!\033[0m Click the widget on your bar to open the popover.")
    print("    Press \033[1;33mCtrl+C\033[0m to quit.\n")

    try:
        app.run()
    except (KeyboardInterrupt, SystemExit):
        pass
    finally:
        _cleanup()


if __name__ == "__main__":
    main()
