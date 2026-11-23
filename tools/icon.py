"""Draws the tray icon (app/icon.png, app/icon.ico): a gold coin with a K."""
import os
from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
S = 256
img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
d = ImageDraw.Draw(img)
d.ellipse((8, 8, S - 8, S - 8), fill=(224, 187, 0), outline=(120, 90, 0), width=14)
d.ellipse((40, 40, S - 40, S - 40), outline=(255, 220, 70), width=8)
try:
    font = ImageFont.truetype("DejaVuSans-Bold.ttf", 140)
except OSError:
    font = ImageFont.load_default()
d.text((S / 2, S / 2 + 4), "K", font=font, fill=(60, 40, 0), anchor="mm")
app = os.path.join(HERE, "..", "app")
img.resize((64, 64), Image.LANCZOS).save(os.path.join(app, "icon.png"))
img.save(os.path.join(app, "icon.ico"), sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (256, 256)])
