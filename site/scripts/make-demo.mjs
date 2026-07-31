/**
 * Generates the landing page's demo assets from the recorded GIF:
 *
 *   site/public/demos/hero.mp4          the recording, minus its blank opening
 *   site/public/demos/hero-poster.png   its first frame, also the reduced-motion still
 *
 * Run with `npm run demo` after re-recording the tape.
 *
 * Both come from START_FRAME, so the poster is exactly the video's first frame
 * and there is no cut when playback starts. Frames before it are the terminal
 * still drawing itself.
 *
 * Needs ffmpeg, which VHS already requires.
 */
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import sharp from 'sharp';

const START_FRAME = 40;
const FPS = 25;

const demos = (name) => fileURLToPath(new URL(`../public/demos/${name}`, import.meta.url));

const gif = demos('hero.gif');
const start = (START_FRAME / FPS).toFixed(2);

execFileSync('ffmpeg', [
	'-hide_banner', '-loglevel', 'error', '-y',
	'-i', gif,
	'-ss', start,
	// H.264 needs even dimensions and yuv420p to play everywhere.
	'-vf', 'scale=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p',
	'-c:v', 'libx264', '-crf', '28', '-preset', 'slow',
	'-movflags', '+faststart', '-an',
	demos('hero.mp4'),
]);

const poster = await sharp(gif, { page: START_FRAME })
	.png({ palette: true, effort: 10 })
	.toFile(demos('hero-poster.png'));

const { size } = await import('node:fs').then((fs) => fs.statSync(demos('hero.mp4')));

console.log(`hero.mp4:        from ${start}s, ${(size / 1024).toFixed(0)} KB`);
console.log(`hero-poster.png: frame ${START_FRAME}, ${poster.width}×${poster.height}, ${(poster.size / 1024).toFixed(0)} KB`);
