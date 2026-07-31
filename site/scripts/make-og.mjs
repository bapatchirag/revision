/**
 * Generates site/public/og.png — the 1200×630 social card.
 *
 * Run with `npm run og`. The PNG is committed; this only needs re-running when
 * the card's wording or the palette changes.
 *
 * Rasterizing needs the mono face installed as a *system* font, because sharp
 * renders SVG through librsvg/fontconfig and cannot read the subset woff2 files
 * in site/public/fonts/. Without it the text falls back to the system monospace,
 * which still lays out correctly but is off-brand.
 *
 * Raw hex is intentional and unavoidable here: an SVG rasterized outside a
 * browser has no CSS custom properties to resolve. Values mirror cipher.css.
 */
import { fileURLToPath } from 'node:url';

import sharp from 'sharp';

const c = {
	bg: '#141d2b',
	frame: '#111927',
	border: '#313f55',
	accent: '#9fef00',
	text: '#e6e9f2',
	textHigh: '#ffffff',
	secondary: '#a4b1cd',
	muted: '#7a7899',
};

const mono = "'Fantasque Sans Mono','FantasqueSansM Nerd Font',monospace";

/** Byte-identical to `revisionLogo` in internal/app/about.go. */
const wordmark = String.raw`             _    _
 _ _ _____ _(_)__(_)___ _ _
| '_/ -_) V / (_-< / _ \ ' \
|_| \___|\_/|_/__/_\___/_||_|`.split('\n');

const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

const logo = wordmark
	.map((line, i) => `<tspan x="110" y="${165 + i * 60}">${esc(line)}</tspan>`)
	.join('');

/* Text nodes stay on one line: xml:space="preserve" would otherwise keep the
   indentation of a wrapped source line as leading whitespace in the render. */
const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
  <rect width="1200" height="630" fill="${c.bg}"/>
  <rect x="60" y="60" width="1080" height="510" rx="12" fill="${c.frame}" stroke="${c.border}" stroke-width="2"/>

  <!-- Title inlaid in the top border, as the TUI panels draw it. -->
  <rect x="104" y="47" width="140" height="26" fill="${c.frame}"/>
  <text x="112" y="67" font-family="${mono}" font-size="20" font-weight="700" fill="${c.accent}" xml:space="preserve">[1]<tspan fill="${c.textHigh}" font-weight="400"> revision</tspan></text>

  <text font-family="${mono}" font-size="52" font-weight="700" fill="${c.accent}" xml:space="preserve">${logo}</text>

  <text x="112" y="420" font-family="${mono}" font-size="28" fill="${c.secondary}">A lazygit-style terminal UI for Subversion</text>

  <text x="112" y="480" font-family="${mono}" font-size="22" fill="${c.accent}" xml:space="preserve">$<tspan fill="${c.text}"> curl -fsSL .../install.sh | sh</tspan></text>

  <!-- Footer inlaid in the bottom border, mirroring the panel count. -->
  <rect x="800" y="557" width="304" height="26" fill="${c.frame}"/>
  <text x="1096" y="577" text-anchor="end" font-family="${mono}" font-size="18" fill="${c.muted}">bapatchirag.github.io/revision</text>
</svg>`;

const out = fileURLToPath(new URL('../public/og.png', import.meta.url));

const info = await sharp(Buffer.from(svg)).png({ compressionLevel: 9 }).toFile(out);

console.log(`og.png: ${info.width}×${info.height}, ${(info.size / 1024).toFixed(0)} KB`);
