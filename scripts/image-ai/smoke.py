"""Bounded real CPU inference smoke; no provider API or credentials."""
import argparse
import io
import json
from pathlib import Path
import urllib.request
from PIL import Image, ImageChops, ImageDraw

parser = argparse.ArgumentParser()
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
args.output.mkdir(parents=True, exist_ok=True)
source = Image.new('RGBA', (256, 256), 'white')
draw = ImageDraw.Draw(source)
for box in [(80, 28, 176, 124), (128, 70, 224, 166), (110, 128, 206, 224), (44, 128, 140, 224), (24, 64, 120, 160)]:
    draw.ellipse(box, fill='#277bd2')
draw.ellipse((86, 86, 166, 166), fill='#f5c441')
source.save(args.output / 'input.png')
buf = io.BytesIO(); source.save(buf, format='PNG')
results = []
for operation, scale in [('removeBackground', 2), ('upscale', 2), ('upscale', 4)]:
    req = urllib.request.Request(f'http://127.0.0.1:18112/process?operation={operation}&scale={scale}', buf.getvalue(), {'Content-Type': 'application/octet-stream'})
    with urllib.request.urlopen(req, timeout=120) as response:
        output = response.read()
    name = f'{operation}-{scale}.png'
    (args.output / name).write_bytes(output)
    image = Image.open(io.BytesIO(output)).convert('RGBA')
    if operation == 'removeBackground':
        alpha = image.getchannel('A')
        assert image.size == source.size and alpha.getextrema()[0] < 30 and alpha.getextrema()[1] > 200
        assert alpha.getpixel((128, 128)) > 200
        info = {'alphaRange': alpha.getextrema()}
    else:
        assert image.size == (source.width * scale, source.height * scale)
        difference = ImageChops.difference(image.convert('RGB'), source.convert('RGB').resize(image.size, Image.Resampling.BICUBIC))
        assert difference.getbbox() is not None, 'AI output must not equal ordinary interpolation'
        info = {'differsFromBicubic': True}
    results.append({'operation': operation, 'scale': scale, 'file': name, 'size': image.size, **info})
(args.output / 'ai-smoke.json').write_text(json.dumps(results, indent=2), encoding='utf-8')
print(json.dumps(results))
