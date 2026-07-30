// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
	site: 'https://bapatchirag.github.io',
	base: '/revision',
	trailingSlash: 'always',
	integrations: [
		starlight({
			title: 'revision',
			description: 'A lazygit-style terminal UI for Subversion (SVN).',
			favicon: '/favicon.svg',
			logo: {
				src: './src/assets/logo-cipher.svg',
				alt: 'revision',
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
			customCss: ['./src/styles/fonts.css', './src/styles/cipher.css'],
			components: {
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
