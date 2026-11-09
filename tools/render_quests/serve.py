"""Static server cho công cụ dựng ảnh nhiệm vụ: phục vụ thư mục gốc repo, đường dẫn không có thì lấy trong public/
(để trang công cụ import được /race/scene.js, /vendor/three/... như trong game).

Dùng: python3 tools/render_quests/serve.py [cổng=8307]
"""
import http.server
import os
import sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))


class Handler(http.server.SimpleHTTPRequestHandler):
    def translate_path(self, path):
        p = super().translate_path(path)
        if os.path.exists(p):
            return p
        rel = os.path.relpath(p, ROOT)
        return os.path.join(ROOT, "public", rel)

    def log_message(self, *a):
        pass


if __name__ == "__main__":
    os.chdir(ROOT)
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8307
    http.server.ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
