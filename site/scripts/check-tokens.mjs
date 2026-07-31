/**
 * Token gate: no raw hex colour may appear in a CSS declaration outside
 * src/styles/cipher.css. Everything else must resolve through a --rv-* token.
 *
 * Scans .css files and the <style> blocks of .astro components, with CSS
 * comments stripped so explanatory prose does not trip the check.
 *
 * Exempt by design:
 *   - src/styles/cipher.css          the token declarations themselves
 *   - src/styles/cipher-code-theme.mjs  a TextMate theme, which cannot use CSS vars
 *   - src/assets/*.svg               static artwork
 */
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const srcDir = fileURLToPath(new URL('../src', import.meta.url));
const exempt = new Set(['styles/cipher.css']);

/** @returns {string[]} */
function walk(dir) {
	return readdirSync(dir).flatMap((entry) => {
		const full = join(dir, entry);
		return statSync(full).isDirectory() ? walk(full) : [full];
	});
}

const stripComments = (css) => css.replace(/\/\*[\s\S]*?\*\//g, '');

const hex = /#[0-9a-fA-F]{3,8}\b/g;
const failures = [];

for (const file of walk(srcDir)) {
	const rel = relative(srcDir, file).replaceAll('\\', '/');
	if (exempt.has(rel)) continue;

	let css;
	if (file.endsWith('.css')) {
		css = readFileSync(file, 'utf8');
	} else if (file.endsWith('.astro')) {
		css = [...readFileSync(file, 'utf8').matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)]
			.map((m) => m[1])
			.join('\n');
	} else {
		continue;
	}

	for (const match of stripComments(css).matchAll(hex)) {
		failures.push(`${rel}: ${match[0]}`);
	}
}

if (failures.length > 0) {
	console.error('Raw hex colours found outside src/styles/cipher.css:');
	for (const failure of failures) console.error(`  ${failure}`);
	console.error('\nUse a --rv-* token instead.');
	process.exit(1);
}

console.log('Token gate: no raw hex outside src/styles/cipher.css.');
