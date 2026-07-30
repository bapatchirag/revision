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
		}),
	],
});
