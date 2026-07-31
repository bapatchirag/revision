/**
 * Generates the site's demo assets.
 *
 *   hero.mp4 / hero-poster.png   cut from the recorded GIF the README embeds
 *   <feature>-poster.png         first frame of each per-feature recording,
 *                                which VHS writes straight to mp4
 *
 * A poster is what the page paints before anything is fetched, and all that is
 * ever shown when the reader has Reduce Motion on, so it is worth a palette pass
 * rather than taking ffmpeg's PNG as-is.
 *
 * Run with `npm run demo` after re-recording, or through `make demos`, which
 * records first.
 *
 * Needs ffmpeg, which VHS already requires.
 */
import { execFileSync } from 'node:child_process';
import { readdirSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import sharp from 'sharp';

const START_FRAME = 40;
const FPS = 25;

const dir = fileURLToPath(new URL('../public/demos/', import.meta.url));
const demos = (name) => `${dir}${name}`;

const kb = (bytes) => `${(bytes / 1024).toFixed(0)} KB`;

const report = (file, detail) => console.log(`${file.padEnd(27)}${detail}`);

/** Writes a palette PNG poster for name and reports its dimensions and size. */
async function poster(name, source, options) {
	const { size, width, height } = await sharp(source, options)
		.png({ palette: true, effort: 10 })
		.toFile(demos(`${name}-poster.png`));
	return `${width}×${height}, ${kb(size)}`;
}

// The hero is the one recording that exists as a GIF first, because the README
// embeds it. Both derivatives come from START_FRAME, so the poster is exactly
// the video's first frame and there is no cut when playback starts. Frames
// before it are the terminal still drawing itself.
const start = (START_FRAME / FPS).toFixed(2);

execFileSync('ffmpeg', [
	'-hide_banner', '-loglevel', 'error', '-y',
	'-i', demos('hero.gif'),
	'-ss', start,
	// H.264 needs even dimensions and yuv420p to play everywhere.
	'-vf', 'scale=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p',
	'-c:v', 'libx264', '-crf', '28', '-preset', 'slow',
	'-movflags', '+faststart', '-an',
	demos('hero.mp4'),
]);

report('hero.mp4', `from ${start}s, ${kb(statSync(demos('hero.mp4')).size)}`);
report('hero-poster.png', await poster('hero', demos('hero.gif'), { page: START_FRAME }));

// The per-feature tapes keep their setup and the app's first paint hidden, so
// frame 0 is already a fully drawn screen.
const features = readdirSync(dir)
	.filter((file) => file.endsWith('.mp4') && file !== 'hero.mp4')
	.map((file) => file.slice(0, -'.mp4'.length))
	.sort();

for (const name of features) {
	const frame = execFileSync(
		'ffmpeg',
		[
			'-hide_banner', '-loglevel', 'error',
			'-i', demos(`${name}.mp4`),
			'-frames:v', '1', '-f', 'image2pipe', '-vcodec', 'png', '-',
		],
		{ maxBuffer: 64 * 1024 * 1024 }
	);
	report(`${name}.mp4`, kb(statSync(demos(`${name}.mp4`)).size));
	report(`${name}-poster.png`, await poster(name, frame));
}

