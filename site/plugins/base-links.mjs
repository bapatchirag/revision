import { defineHastPlugin } from 'satteri';

/**
 * Prefixes site-absolute links in Markdown and MDX with the configured base.
 *
 * Content is written with hrefs like `/guides/panels/`. Astro does not rewrite
 * those, and the site is served from `/revision`, so without this every
 * cross-link 404s once deployed — invisible under `astro dev`, which serves
 * from the base too, but fatal on GitHub Pages.
 *
 * Doing it here keeps the base out of the content files entirely.
 *
 * @param {string} base
 */
export function baseLinks(base) {
	const prefix = base.replace(/\/$/, '');

	return defineHastPlugin({
		name: 'revision-base-links',
		element: [
			{
				filter: ['a'],
				visit(node, ctx) {
					const href = node.properties?.href;
					if (typeof href !== 'string') return;
					// Site-absolute only. Protocol-relative, external and in-page links are left alone.
					if (!href.startsWith('/') || href.startsWith('//')) return;
					if (href === prefix || href.startsWith(`${prefix}/`)) return;
					ctx.setProperty(node, 'href', prefix + href);
				},
			},
		],
	});
}
