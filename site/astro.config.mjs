// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { satteri } from '@astrojs/markdown-satteri';

import { baseLinks } from './plugins/base-links.mjs';

const SITE = 'https://bapatchirag.github.io';
const BASE = '/revision';

export default defineConfig({
	site: SITE,
	base: BASE,
	trailingSlash: 'always',
	markdown: {
		// Astro's default processor, plus the one plugin. Content links are written
		// site-absolute; this puts the base in front of them.
		processor: satteri({ hastPlugins: [baseLinks(BASE)] }),
	},
	integrations: [
		starlight({
			title: 'revision',
			description: 'A lazygit-style terminal UI for Subversion (SVN).',
			favicon: '/favicon.svg',
			logo: {
				src: './src/assets/logo-cipher.svg',
				// Decorative: the site title sits right beside it and already says "revision".
				alt: '',
			},
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/bapatchirag/revision',
				},
			],
			editLink: {
				baseUrl: 'https://github.com/bapatchirag/revision/edit/main/site/',
			},
			// The 404 page is src/pages/404.astro; without this both routes claim /404.
			disable404Route: true,
			sidebar: [
				{
					label: 'Guides',
					// Collapsed by default; Starlight still opens the group holding the current page.
					collapsed: true,
					items: [
						{ label: 'Introduction', link: '/guides/introduction/' },
						{ label: 'Requirements', link: '/guides/requirements/' },
						{ label: 'Installation', link: '/guides/installation/' },
						{ label: 'Quick start', link: '/guides/quick-start/' },
						{ label: 'The panels', link: '/guides/panels/' },
					],
				},
				{
					label: 'Workflows',
					collapsed: true,
					items: [
						{ label: 'Staging & commit', link: '/workflows/staging-and-commit/' },
						{ label: 'Changelists', link: '/workflows/changelists/' },
						{ label: 'Working with diffs', link: '/workflows/diffs/' },
						{ label: 'Updating the working copy', link: '/workflows/updating/' },
						{ label: 'Resolving conflicts & rejects', link: '/workflows/conflicts/' },
						{ label: 'Filtering & searching', link: '/workflows/filtering-and-searching/' },
						{ label: 'Opening files in an editor', link: '/workflows/editor/' },
					],
				},
				{
					label: 'Reference',
					collapsed: true,
					items: [
						{ label: 'Keybindings', link: '/reference/keybindings/' },
						{ label: 'Configuration', link: '/reference/configuration/' },
						{ label: 'Themes', link: '/reference/themes/' },
						{ label: 'CLI flags', link: '/reference/cli/' },
					],
				},
				{
					label: 'Operations',
					collapsed: true,
					items: [
						{ label: 'Authentication (svn+ssh)', link: '/operations/authentication/' },
						{ label: 'Updating revision', link: '/operations/updating-revision/' },
						{ label: 'Troubleshooting', link: '/operations/troubleshooting/' },
					],
				},
				{
					label: 'Develop',
					collapsed: true,
					items: [
						{ label: 'Architecture', link: '/develop/architecture/' },
						{ label: 'Building from source', link: '/develop/building/' },
						{ label: 'Contributing', link: '/develop/contributing/' },
						{ label: 'Regenerating demos', link: '/develop/demos/' },
					],
				},
				{ label: 'FAQ', link: '/faq/' },
			],
			customCss: ['./src/styles/fonts.css', './src/styles/cipher.css'],
			components: {
				// The landing page is the only page with a hero; the wordmark is its title.
				Hero: './src/components/Hero.astro',
				// The default social links, plus the GitHub repository actions.
				SocialIcons: './src/components/SocialIcons.astro',
				// Dark-only site: the theme toggle is overridden away.
				ThemeSelect: './src/components/ThemeSelect.astro',
			},
			// Expressive Code is configured in ec.config.mjs.
			head: [
				{
					tag: 'link',
					attrs: {
						rel: 'preload',
						href: '/revision/fonts/FantasqueSansMono-Regular.subset.woff2',
						as: 'font',
						type: 'font/woff2',
						crossorigin: 'anonymous',
					},
				},
				// Starlight emits og:title, og:description and og:url itself.
				{ tag: 'meta', attrs: { property: 'og:image', content: `${SITE}${BASE}/og.png` } },
				{ tag: 'meta', attrs: { property: 'og:image:width', content: '1200' } },
				{ tag: 'meta', attrs: { property: 'og:image:height', content: '630' } },
				{
					tag: 'meta',
					attrs: {
						property: 'og:image:alt',
						content: 'revision — a lazygit-style terminal UI for Subversion',
					},
				},
				{ tag: 'meta', attrs: { name: 'twitter:card', content: 'summary_large_image' } },
				{ tag: 'meta', attrs: { name: 'twitter:image', content: `${SITE}${BASE}/og.png` } },
			],
		}),
	],
	vite: {
		server: {
			// style.astro imports the Go golden file from the repo root with ?raw.
			fs: { allow: ['..'] },
		},
	},
});
