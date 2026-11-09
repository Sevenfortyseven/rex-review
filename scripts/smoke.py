#!/usr/bin/env python3
"""Exercise the real UI on a disposable Rex server; never talks to an actual agent."""
import json
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
REX = os.environ.get("REX_REVIEW_REX") or shutil.which("rex") or "/Applications/Rex Beta.app/Contents/Helpers/Rex Server.rex/Contents/MacOS/rex"


def main():
    assert (ROOT / "bin/rex-review").exists(), "Build bin/rex-review first"
    with tempfile.TemporaryDirectory(prefix="rex-review-smoke-") as folder:
        tmp = Path(folder)
        endpoint = "unix://" + str(tmp / "server.sock")
        config = tmp / "init.lua"
        config.write_text(
            'rexd = {plugin = {dir = %s, options = {server = %s}}}\ndofile(%s)\n'
            % (json.dumps(str(ROOT)), json.dumps(endpoint), json.dumps(str(ROOT / "rex/config.lua")))
        )
        # A raw-input recorder named codex exercises process detection and literal paste.
        # It cannot execute received input, and logs only test-generated comments.
        received = tmp / "received.txt"
        source = tmp / "codex.c"
        source.write_text(r'''
#include <stdio.h>
#include <unistd.h>
#include <termios.h>
int main(int argc, char **argv) {
    struct termios t; tcgetattr(0, &t); cfmakeraw(&t); tcsetattr(0, TCSANOW, &t);
    printf("\033[?2004hFake agent ready\r\n"); fflush(stdout);
    FILE *f = fopen(argv[1], "ab"); char b[4096]; ssize_t n;
    while ((n = read(0, b, sizeof b)) > 0) { fwrite(b, 1, n, f); fflush(f); }
    return 0;
}
''')
        subprocess.run(["cc", str(source), "-o", str(tmp / "codex")], check=True)
        repo = tmp / "repo"
        repo.mkdir()
        file = repo / "example.swift"
        before = 'func greeting() -> String {\n    return "Hello"\n}\n'
        after = before.replace('"Hello"', '"Hello, Rex"')
        file.write_text(before)
        for args in [["init", "-q"], ["config", "user.name", "Fixture"], ["config", "user.email", "fixture@example.test"], ["add", "."], ["commit", "-qm", "baseline"]]:
            subprocess.run(["git", "-C", str(repo)] + args, check=True)
        file.write_text(after)

        # Test actual colored rendering even when the invoking automation sets NO_COLOR.
        env = {k: v for k, v in os.environ.items() if k not in ("REX_SERVER", "REX_SESSION", "REX_BLOCK", "NO_COLOR")}
        log = (tmp / "server.log").open("w")
        server = subprocess.Popen([REX, "server", "run", "--listen", endpoint, "--state-dir", str(tmp / "state"), "--config", str(config), "--restore=false"], env=env, stdout=log, stderr=log)

        def call(*args):
            return subprocess.check_output([REX, "--autostart=false", "-S", endpoint] + list(args), text=True, stderr=subprocess.STDOUT, timeout=20)

        draft_path = None
        hint_path = None
        try:
            deadline = time.monotonic() + 10
            while not (tmp / "server.sock").exists():
                if server.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError((tmp / "server.log").read_text())
                time.sleep(.05)
            # Launch outside the repository, as when an agent changes its logical
            # workspace without changing its OS cwd.
            created = json.loads(call("new", "rex-review-smoke", "-c", str(tmp), "--shell=none", "--json", "--", str(tmp / "codex"), str(received)))
            session = created["session_id"]
            agent = created["initial_windows"][0]["block_ids"][0]
            config_base = Path.home() / "Library/Application Support" if sys.platform == "darwin" else Path(os.environ.get("XDG_CONFIG_HOME", str(Path.home() / ".config")))
            identity = str(repo.resolve()) + "\0" + session + "\0" + agent
            draft_path = config_base / "rex-review/drafts" / (hashlib.sha256(identity.encode()).hexdigest() + ".json")
            process = json.loads(call("block", "call", "-s", session, "-b", agent, "process"))["foreground"]
            hint_identity = session + "\0" + agent + "\0" + str(process["pid"])
            hint_path = config_base / "rex-review/repositories" / (hashlib.sha256(hint_identity.encode()).hexdigest() + ".json")
            project_prompted = False

            def open_review():
                nonlocal project_prompted
                layer = json.loads(call("do", "rex_review", "-s", session, "-b", agent, "--json"))
                review = layer["block_ids"][0]
                deadline = time.monotonic() + 10
                while True:
                    text = screen(review)
                    if "file entries" in text:
                        break
                    if "Choose project folder" in text:
                        assert not project_prompted, "project choice was not remembered"
                        # A non-Git directory should show an editable error, not exit.
                        keys(review, "enter")
                        wait_screen(review, "not an accessible Git working tree")
                        keys(review, "ctrl+u")
                        call("api", "call", "block.event", json.dumps({"session_id": session, "block_id": review, "event": {"type": "paste", "payload": {"text": str(repo)[:-2]}}}))
                        keys(review, "tab")
                        wait_screen(review, "Folder completed")
                        keys(review, "enter")
                        project_prompted = True
                    if time.monotonic() > deadline:
                        raise RuntimeError(call("capture", "-s", session, "-b", review, "--trim"))
                    time.sleep(.05)
                return review

            def keys(review, *values):
                call("send-key", "-s", session, "-b", review, *values)

            def screen(review):
                return call("capture", "-s", session, "-b", review, "--trim")

            def wait_screen(review, text):
                deadline = time.monotonic() + 10
                while text not in screen(review):
                    if time.monotonic() > deadline:
                        raise AssertionError(screen(review))
                    time.sleep(.05)

            review = open_review()
            appearance = json.loads(subprocess.check_output([str(ROOT / "bin/rex-review"), "theme"], text=True, stderr=subprocess.DEVNULL))
            if appearance.get("palette"):
                # The title uses ANSI cyan. Its rendered color must come from
                # Rex's chosen theme rather than the generic terminal palette.
                cyan = next(color.split("=", 1)[1] for color in appearance["palette"] if color.startswith("6="))
                html = call("capture", "-s", session, "-b", review, "--format=html")
                assert "var(--vt-palette-6)" in html, "review title did not use the terminal's ANSI palette"
                formatted = json.loads(call("block", "call", "-s", session, "-b", review, "format", json.dumps({"format": "vt", "extra_palette": True, "max_bytes": 32768})))
                expected = "4;6;rgb:" + "/".join(cyan[i:i+2].lower() for i in (1, 3, 5))
                assert expected in formatted["content"].lower(), "floating terminal did not receive the selected Rex palette"
            keys(review, *(["down"] * 7 + ["c"]))
            call("api", "call", "block.event", json.dumps({"session_id": session, "block_id": review, "event": {"type": "paste", "payload": {"text": "Use a localized greeting here."}}}))
            keys(review, "ctrl+s")
            wait_screen(review, "Comment saved")
            # Closing via the real Lua action must preserve drafts.
            call("do", "rex_review", "-s", session, "-b", review, "--json")
            review = open_review()
            keys(review, "shift+c")
            wait_screen(review, "Use a localized greeting here.")
            file.write_text(after.replace("Hello, Rex", "Changed again"))
            keys(review, "s", "y")
            wait_screen(review, "changed since you commented")
            assert not received.exists() or received.stat().st_size == 0
            file.write_text(after)
            keys(review, "s", "y")
            deadline = time.monotonic() + 10
            while not received.exists() or b"\x1b[201~" not in received.read_bytes():
                assert time.monotonic() < deadline, "review was not delivered"
                time.sleep(.05)
            message = received.read_text()
            assert "example.swift:2 (unstaged, new-file lines)" in message
            assert "Use a localized greeting here." in message
            assert message.count("Please address these review comments") == 1
            assert "\x1b[200~" in message and "\x1b[201~" in message
            deadline = time.monotonic() + 5
            while True:
                view = json.loads(call("session", "inspect", session, "--json"))
                blocks = [b["block_id"] for w in view["windows"] for l in w["layers"] for b in l["blocks"]]
                if review not in blocks:
                    break
                assert time.monotonic() < deadline, "review did not close after send"
                time.sleep(.05)
            print("PASS: Rex theme palette, non-Git launch recovery, Tab completion, remembered project, draft recovery, stale refusal, single delivery")
        finally:
            try:
                call("server", "stop")
            except (subprocess.SubprocessError, OSError):
                server.terminate()
            try:
                server.wait(timeout=5)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait()
            log.close()
            if draft_path is not None and draft_path.exists():
                draft_path.unlink()
            if hint_path is not None and hint_path.exists():
                hint_path.unlink()


if __name__ == "__main__":
    main()
