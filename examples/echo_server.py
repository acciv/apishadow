"""Демо-апстрим: простой JSON API для primary и shadow."""

import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

ROLE = os.environ.get("ROLE", "primary")
PORT = int(os.environ.get("PORT", "9101"))


class Handler(BaseHTTPRequestHandler):
    def _respond(self):
        length = int(self.headers.get("Content-Length", "0"))
        echo = self.rfile.read(length).decode("utf-8", "replace")[:120]

        payload = {
            "role": ROLE,
            "method": self.command,
            "path": self.path,
            "version": "1.0" if ROLE == "primary" else "2.0",
            "items": [1, 2, 3],
            "timestamp": "volatile-value",
            "echo": echo,
        }
        if ROLE == "shadow":
            payload["debug"] = True

        status = 500 if self.path.startswith("/v1/error") and ROLE == "shadow" else 200
        data = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Date", "Mon, 01 Jan 2024 00:00:00 GMT")
        self.end_headers()
        self.wfile.write(data)

    do_GET = _respond
    do_POST = _respond

    def log_message(self, *_args):
        pass


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
