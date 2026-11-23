"""Explicit model installation; the inference server never downloads models."""
import argparse
import hashlib
import json
from pathlib import Path
import urllib.request

MODELS = {
    'u2netp.onnx': ('https://github.com/danielgatis/rembg/releases/download/v0.0.0/u2netp.onnx', '8e83ca70e441ab06c318d82300c84806'),
    'FSRCNN_x2.pb': ('https://raw.githubusercontent.com/Saafke/FSRCNN_Tensorflow/master/models/FSRCNN_x2.pb', ''),
    'FSRCNN_x4.pb': ('https://raw.githubusercontent.com/Saafke/FSRCNN_Tensorflow/master/models/FSRCNN_x4.pb', ''),
}
SHA256 = {
    'u2netp.onnx': '309c8469258dda742793dce0ebea8e6dd393174f89934733ecc8b14c76f4ddd8',
    'FSRCNN_x2.pb': '366b33f0084c7b3f2bf6724f0a2c77bca94fcec9d7b6d72389d330073b380d5c',
    'FSRCNN_x4.pb': '5c68d18db561aed8ead4ffedf1b897ea615baaf60ebf6c35f8e641f8fa4a21bf',
}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--models', type=Path, required=True)
    args = parser.parse_args()
    args.models.mkdir(parents=True, exist_ok=True)
    manifest = {}
    for name, (url, md5) in MODELS.items():
        path = args.models / name
        data = path.read_bytes() if path.is_file() else urllib.request.urlopen(url, timeout=60).read(10 << 20)
        if len(data) < 1000 or hashlib.sha256(data).hexdigest() != SHA256[name] or (md5 and hashlib.md5(data).hexdigest() != md5):
            raise SystemExit('Model verification failed: ' + name)
        path.write_bytes(data)
        manifest[name] = {'source': url, 'sha256': hashlib.sha256(data).hexdigest(), 'bytes': len(data)}
        print(name, len(data), manifest[name]['sha256'])
    (args.models / 'manifest.json').write_text(json.dumps(manifest, indent=2), encoding='utf-8')


if __name__ == '__main__':
    main()
