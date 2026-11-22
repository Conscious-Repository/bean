"""Exercise the reader with a server independent of the host recipe."""
import hashlib
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
import subprocess
import sys
import threading
import unittest

BODY = b"example pack bytes\n"
DIGEST = hashlib.sha256(BODY).hexdigest()


class ReaderTest(unittest.TestCase):
    def run_check(self, head=200, get=200, body=BODY, length=None):
        requests = []

        class Server(BaseHTTPRequestHandler):
            def reply(self, status):
                requests.append((self.command, self.path, self.headers.get("Authorization")))
                self.send_response(status)
                if length is not None:
                    self.send_header("Content-Length", str(length))
                self.end_headers()

            def do_HEAD(self):
                self.reply(head)

            def do_GET(self):
                self.reply(get)
                self.wfile.write(body)

            def log_message(self, *args):
                pass

        with HTTPServer(("127.0.0.1", 0), Server) as server:
            thread = threading.Thread(target=server.serve_forever)
            thread.start()
            try:
                result = subprocess.run(
                    [sys.executable, Path(__file__).with_name("check.py"),
                     f"http://127.0.0.1:{server.server_port}", DIGEST],
                    capture_output=True, timeout=10,
                )
            finally:
                server.shutdown()
                thread.join()
        return result, requests

    def test_verified_bytes(self):
        result, requests = self.run_check()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, BODY)
        self.assertEqual(requests, [("HEAD", "/" + DIGEST, None), ("GET", "/" + DIGEST, None)])

    def test_rejects_wrong_bytes_or_status(self):
        for args in ({"body": BODY + b"!"}, {"head": 405}, {"get": 302}, {"get": 404}, {"length": len(BODY) + 1}):
            with self.subTest(args=args):
                result, _ = self.run_check(**args)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, b"")


if __name__ == "__main__":
    unittest.main()
