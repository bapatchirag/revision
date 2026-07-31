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
						{ label: 'VS Code extension', link: '/reference/vscode-extension/' },
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
			],
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
