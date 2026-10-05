#!/usr/bin/env python3
"""验收用 webhook 接收器：记录每个 POST /hook（时间戳/鉴权头/正文）到 JSONL。"""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = sys.argv[2] if len(sys.argv) > 2 else ".acceptance/logs/webhooks.jsonl"
EXPECT_SECRET = sys.argv[1] if len(sys.argv) > 1 else ""
# 端口缺省 9700（notify 先例）；server-backup 等并行域传 argv[3] 错开
PORT = int(sys.argv[3]) if len(sys.argv) > 3 else 9700


class Hook(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length).decode("utf-8", "replace")
        secret = self.headers.get("X-Cockpit-Secret", "")
        ok = (not EXPECT_SECRET) or secret == EXPECT_SECRET
        with open(LOG, "a") as f:
            f.write(json.dumps({
                "ts": self.date_time_string(),
                "path": self.path,
                "secret_ok": ok,
                "body": body,
            }) + "\n")
        self.send_response(200 if ok else 401)
        self.end_headers()
        self.wfile.write(b"ok")

    def log_message(self, *args):
        pass


HTTPServer(("127.0.0.1", PORT), Hook).serve_forever()
