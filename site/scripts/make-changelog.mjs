/**
 * Writes src/data/releases.json from the repository's GitHub releases, so the
 * changelog page renders from a committed file rather than calling the API on
 * every build — CI builds every pull request, and an unauthenticated runner is
 * rate-limited long before that finishes.
 *
 * Run `make site-changelog` after publishing a release.
 *
 * GITHUB_TOKEN is used when set; the anonymous allowance is enough otherwise.
 */
import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const REPO = 'bapatchirag/revision';
const out = fileURLToPath(new URL('../src/data/releases.json', import.meta.url));

const headers = {
	accept: 'application/vnd.github+json',
	'x-github-api-version': '2022-11-28',
};
if (process.env.GITHUB_TOKEN) headers.authorization = `Bearer ${process.env.GITHUB_TOKEN}`;

const response = await fetch(`https://api.github.com/repos/${REPO}/releases?per_page=100`, {
	headers,
});
if (!response.ok) {
	console.error(`make-changelog: GitHub API said ${response.status} ${response.statusText}`);
	process.exit(1);
}

/**
 * goreleaser writes a `## Changelog` heading over a flat list of `* message
 * (@login)`. Releases up to v1.3.0 predate that format and carry the commit SHA,
 * the merge commits and the author's email address, none of which say anything
 * about the release.
 *
 * @param {string | null | undefined} body
 * @returns {string[]}
 */
const entries = (body) =>
	(body ?? '')
		.split('\n')
		.filter((line) => /^\s*[*-]\s+/.test(line))
		.map((line) =>
			line
				.replace(/^\s*[*-]\s+/, '')
				.replace(/^[0-9a-f]{40}:\s*/, '')
				.replace(/\s*<[^>]*>/g, '')
				.trim(),
		)
		.filter((line) => line !== '' && !line.startsWith('Merge pull request '));

const releases = (await response.json())
	.filter((release) => !release.draft)
	.map((release) => ({
		tag: release.tag_name,
		name: release.name || release.tag_name,
		date: release.published_at,
		url: release.html_url,
		prerelease: release.prerelease,
		entries: entries(release.body),
	}));

writeFileSync(out, `${JSON.stringify(releases, null, 2)}\n`);
console.log(`make-changelog: wrote ${releases.length} releases to src/data/releases.json`);
