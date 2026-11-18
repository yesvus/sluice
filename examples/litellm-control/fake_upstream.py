"""Synthetic OpenAI-compatible upstream fixture for local LiteLLM control demos. Not a real provider."""
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MODELS = {
    "object": "list",
    "data": [
        {"id": "fake-model-a", "object": "model"},
        {"id": "fake-model-b", "object": "model"},
    ],
}


class Handler(BaseHTTPRequestHandler):
    def _send(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.rstrip("/") == "/v1/models":
            self._send(200, MODELS)
        else:
            self._send(404, {"error": {"message": "not found"}})

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0) or 0)
        raw = self.rfile.read(length) if length else b""
        try:
            model = str(json.loads(raw or b"{}").get("model", "unknown"))
        except (ValueError, AttributeError):
            model = "unknown"
        self._send(
            200,
            {
                "id": "chatcmpl-fake-0001",
                "object": "chat.completion",
                "created": 0,
                "model": model,
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": f"served-by:{model}"},
                        "finish_reason": "stop",
                    }
                ],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
            },
        )


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
