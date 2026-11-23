"""Local, stateless CPU inference. Models must be installed before startup."""
import argparse
import io
import hashlib
import json
import os
from pathlib import Path
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

MAX_BYTES = 20 << 20
MAX_PIXELS = 24_000_000
gate = threading.Lock()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--models', type=Path, required=True)
    parser.add_argument('--port', type=int, default=18112)
    args = parser.parse_args()
    models = args.models.resolve()
    from setup_models import SHA256
    for name in ['u2netp.onnx', 'FSRCNN_x2.pb', 'FSRCNN_x4.pb']:
        if not (models / name).is_file():
            raise SystemExit('Missing model: ' + name + '. Run setup_models.py first.')
        if hashlib.sha256((models / name).read_bytes()).hexdigest() != SHA256[name]:
            raise SystemExit('Model checksum mismatch: ' + name)
    os.environ['U2NET_HOME'] = str(models)
    os.environ['OMP_NUM_THREADS'] = '2'
    import cv2
    import numpy as np
    from PIL import Image, ImageOps
    from rembg import new_session, remove
    cv2.setNumThreads(2)
    Image.MAX_IMAGE_PIXELS = MAX_PIXELS
    session = new_session('u2netp', providers=['CPUExecutionProvider'])
    upscalers = {}
    for scale in (2, 4):
        sr = cv2.dnn_superres.DnnSuperResImpl_create()
        sr.readModel(str(models / f'FSRCNN_x{scale}.pb'))
        sr.setModel('fsrcnn', scale)
        upscalers[scale] = sr

    def process(raw, operation, scale):
        with Image.open(io.BytesIO(raw)) as opened:
            if opened.width * opened.height > MAX_PIXELS or max(opened.size) > 8192:
                raise ValueError('Image too large')
            original = ImageOps.exif_transpose(opened).convert('RGBA')
        if operation == 'removeBackground':
            # Preserve existing alpha as well as the inferred foreground mask.
            result = remove(original, session=session)
            alpha = np.minimum(np.asarray(original.getchannel('A')), np.asarray(result.getchannel('A')))
            result.putalpha(Image.fromarray(alpha))
        elif operation == 'upscale' and scale in upscalers:
            w, h = original.size
            if w * h * scale * scale > MAX_PIXELS or max(w, h) * scale > 8192:
                raise ValueError('Output too large')
            rgb = np.asarray(original.convert('RGB'))
            result_rgb = np.zeros((h * scale, w * scale, 3), dtype=np.uint8)
            # Overlapping tiles bound network activation memory and avoid edge seams.
            tile, pad = 192, 12
            for y in range(0, h, tile):
                for x in range(0, w, tile):
                    left, top, right, bottom = max(0, x-pad), max(0, y-pad), min(w, x+tile+pad), min(h, y+tile+pad)
                    patch = cv2.cvtColor(rgb[top:bottom, left:right], cv2.COLOR_RGB2BGR)
                    enlarged = cv2.cvtColor(upscalers[scale].upsample(patch), cv2.COLOR_BGR2RGB)
                    tw, th = min(tile, w-x), min(tile, h-y)
                    ox, oy = (x-left)*scale, (y-top)*scale
                    result_rgb[y*scale:(y+th)*scale, x*scale:(x+tw)*scale] = enlarged[oy:oy+th*scale, ox:ox+tw*scale]
            result = Image.fromarray(result_rgb).convert('RGBA')
            result.putalpha(original.getchannel('A').resize(result.size, Image.Resampling.LANCZOS))
        else:
            raise ValueError('Unsupported operation')
        buf = io.BytesIO()
        result.save(buf, format='PNG')
        if buf.tell() > MAX_BYTES:
            raise ValueError('Output too large')
        return buf.getvalue()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def setup(self):
            super().setup()
            self.connection.settimeout(120)

        def reply(self, code, data, media='application/json'):
            self.send_response(code)
            self.send_header('Content-Type', media)
            self.send_header('Content-Length', str(len(data)))
            self.send_header('Cache-Control', 'no-store')
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if self.path != '/health':
                return self.reply(404, b'{}')
            self.reply(200, json.dumps({'ready': True, 'background': 'u2netp', 'upscale': 'fsrcnn', 'scales': [2, 4]}).encode())

        def do_POST(self):
            # Browser-originated calls are not part of this private worker protocol.
            if self.headers.get('Origin') or self.headers.get('Content-Type') != 'application/octet-stream':
                return self.reply(403, b'{}')
            query = urlsplit(self.path)
            if query.path != '/process':
                return self.reply(404, b'{}')
            if not gate.acquire(blocking=False):
                return self.reply(429, b'{"error":"busy"}')
            try:
                length = int(self.headers.get('Content-Length', '0'))
                if length < 1 or length > MAX_BYTES or self.headers.get('Transfer-Encoding'):
                    return self.reply(413, b'{}')
                params = parse_qs(query.query)
                raw = self.rfile.read(length)
                if len(raw) != length:
                    return self.reply(400, b'{}')
                data = process(raw, params.get('operation', [''])[0], int(params.get('scale', ['2'])[0]))
                self.reply(200, data, 'image/png')
            except Exception:
                self.reply(422, b'{"error":"processingFailed"}')
            finally:
                gate.release()

    server = ThreadingHTTPServer(('127.0.0.1', args.port), Handler)
    server.daemon_threads = True
    print('POD image AI ready on 127.0.0.1:' + str(args.port), flush=True)
    server.serve_forever()


if __name__ == '__main__':
    main()
