#!/usr/bin/env python3
"""Exercise a built app in temporary folders against a local fake member service.

Usage: python3 scripts/check_portable.py /absolute/path/to/Takealot
No Takealot API, real member account, or existing workspace is accessed.
"""
import hashlib
import http.server
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request


class MemberAPI(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        assert self.headers.get("Authorization") == "Bearer portable-test-session"
        assert not self.headers.get("Cookie")
        if self.path == "/api/v1/me":
            data = {"id": 1, "username": "portable-test"}
        elif self.path == "/api/v1/me/memberships":
            data = [{"slug": "takealot-vip", "status": "active"}]
        else:
            raise AssertionError("unexpected member request")
        body = json.dumps({"data": data}).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


def request(base, path, value=None):
    req = urllib.request.Request(base + path)
    if value is not None:
        req = urllib.request.Request(base + path, json.dumps(value).encode(),
                                     {"Content-Type": "application/json"}, method="PUT")
    with urllib.request.urlopen(req, timeout=3) as response:
        return json.load(response)


def check(binary):
    member = http.server.ThreadingHTTPServer(("127.0.0.1", 0), MemberAPI)
    threading.Thread(target=member.serve_forever, daemon=True).start()
    cms = "http://127.0.0.1:" + str(member.server_port)
    processes = []
    logs = []
    try:
        with tempfile.TemporaryDirectory(prefix="takealot-portable-test-") as temporary:
            root = Path(temporary)
            cwd = root / "unrelated-launch-directory"
            cwd.mkdir()
            (cwd / "config.json").write_text('{"authorization":"must-not-import"}')
            a = root / "copy-a"
            a.mkdir()
            exe = "Takealot.exe" if os.name == "nt" else "Takealot"
            shutil.copy2(binary, a / exe)
            account = a / "data" / "accounts"
            account.mkdir(parents=True)
            key = hashlib.sha256((cms + "|takealot-vip").encode()).hexdigest()
            (account / (key + ".json")).write_text('{"access_token":"portable-test-session"}')

            def start(folder):
                log = open(folder / "check-output.log", "wb")
                logs.append(log)
                proc = subprocess.Popen([str(folder / exe), "--server-only", "--no-open", "--gocms-url", cms],
                                        cwd=cwd, stdout=log, stderr=log)
                processes.append(proc)
                deadline = time.monotonic() + 15
                while time.monotonic() < deadline:
                    if proc.poll() is not None:
                        raise AssertionError("app exited during startup: " + str(folder))
                    try:
                        base = json.loads((folder / "data" / "runtime.json").read_text())["url"]
                        if request(base, "/api/account/status")["eligible"]:
                            return proc, base
                    except (OSError, ValueError, KeyError):
                        pass
                    time.sleep(0.1)
                raise AssertionError("startup timed out")

            def stop(proc):
                proc.terminate()
                proc.wait(timeout=10)
                assert proc.returncode == 0

            first, base_a = start(a)
            assert request(base_a, "/api/stores")["stores"] == []
            request(base_a, "/api/preferences", {"theme": "dark", "active_store_id": "shop-a"})
            with sqlite3.connect(a / "data" / "takealot.db") as connection:
                connection.execute("INSERT INTO stores(id,name,authorization) VALUES('shop-a','Test A','test-local-only')")
            stop(first)
            b = root / "copy-b"
            shutil.copytree(a, b)
            first, base_a = start(a)
            second, base_b = start(b)
            assert base_a != base_b
            assert request(base_b, "/api/preferences")["active_store_id"] == "shop-a"
            assert request(base_b, "/api/stores")["stores"][0]["name"] == "Test A"
            request(base_b, "/api/preferences", {"theme": "light", "active_store_id": "copy-b-choice"})
            assert request(base_a, "/api/preferences")["theme"] == "dark"
            duplicate = subprocess.run([str(a / exe), "--server-only", "--no-open", "--gocms-url", cms],
                                       cwd=cwd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)
            assert duplicate.returncode != 0 and "已被其他进程使用" in duplicate.stdout.decode()
            stop(second)
            with sqlite3.connect(b / "data" / "takealot.db") as connection:
                connection.execute("UPDATE stores SET name='Only B' WHERE id='shop-a'")
            second, base_b = start(b)
            assert request(base_b, "/api/stores")["stores"][0]["name"] == "Only B"
            assert request(base_a, "/api/stores")["stores"][0]["name"] == "Test A"
            stop(second)
            stop(first)
            assert not (cwd / "takealot.db").exists()
            assert not (cwd / "data").exists()
            print("PASS: copied login/database/preferences, independent processes/ports/writes, duplicate lock, clean restart, unrelated CWD ignored")
    finally:
        for proc in processes:
            if proc.poll() is None:
                proc.terminate()
                proc.wait(timeout=10)
        for log in logs:
            log.close()
        member.shutdown()
        member.server_close()


if __name__ == "__main__":
    check(Path(sys.argv[1]).resolve())
