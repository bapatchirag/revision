// @ts-check
import { defineEcConfig } from '@astrojs/starlight/expressive-code';

import { cipherCodeTheme } from './src/styles/cipher-code-theme.mjs';

/*
 * Expressive Code lives in its own config file because `themeCssSelector` is a
 * function, and the <Code> component requires astro.config.mjs options to be
 * JSON-serializable.
 */
export default defineEcConfig({
	themes: [cipherCodeTheme],
	useDarkModeMediaQuery: false,
	// Dark-only: emit the theme at the root instead of behind a [data-theme] selector.
	themeCssSelector: () => false,
	styleOverrides: {
		borderColor: 'var(--rv-border)',
		borderRadius: 'var(--rv-radius)',
		codeFontFamily: 'var(--rv-font-code)',
		uiFontFamily: 'var(--rv-font-chrome)',
		frames: {
			editorTabBarBackground: 'var(--rv-bg-elevated)',
			editorActiveTabBackground: 'var(--rv-bg-deep)',
			editorActiveTabIndicatorBottomColor: 'var(--rv-accent)',
			terminalTitlebarBackground: 'var(--rv-bg-elevated)',
			terminalBackground: 'var(--rv-bg-deep)',
		},
	},
});
