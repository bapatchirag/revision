/**
 * Syntax theme for Expressive Code / Shiki.
 *
 * Authored from the --rv-* palette in src/styles/cipher.css so code blocks sit
 * in the same colour family as the rest of the site. It is NOT a copy of the
 * Cipher VS Code extension's `tokenColors`: that extension ships under the
 * eXPL licence, which forbids redistribution, so its token table is not
 * vendored here. See SITE-LOG.md, "Deviations from plan".
 *
 * This file and src/styles/cipher.css are the only places raw hex may appear.
 */

const bgDeep = '#111927';
const bg = '#141d2b';
const bgElevated = '#1a2332';
const bgSelection = '#302540';
const text = '#e6e9f2';
const textSecondary = '#a4b1cd';
const muted = '#7a7899';
const comment = '#d7e4ff45';
const border = '#313f55';
const accent = '#9fef00';
const error = '#ff3e3e';
const warning = '#ffaf00';
const info = '#4dd2e1';
const magenta = '#c88dea';
const blue = '#5cb2ff';

/** @type {import('shiki').ThemeRegistration} */
export const cipherCodeTheme = {
	name: 'cipher-dark',
	type: 'dark',
	colors: {
		'editor.background': bgDeep,
		'editor.foreground': text,
		'editor.lineHighlightBackground': bgSelection,
		'editor.selectionBackground': bgSelection,
		'editorLineNumber.foreground': muted,
		'editorLineNumber.activeForeground': accent,
		'editorGroup.border': border,
		'editorWidget.background': bgElevated,
		'panel.background': bgDeep,
		'terminal.background': bgDeep,
		'activityBar.background': bg,
		focusBorder: accent,
	},
	tokenColors: [
		{ scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: comment, fontStyle: 'italic' } },
		{
			scope: ['keyword', 'storage', 'storage.type', 'keyword.control', 'keyword.operator.new', 'variable.language'],
			settings: { foreground: magenta },
		},
		{ scope: ['keyword.operator', 'punctuation', 'meta.brace'], settings: { foreground: textSecondary } },
		{ scope: ['string', 'string.quoted', 'punctuation.definition.string'], settings: { foreground: accent } },
		{ scope: ['constant.numeric', 'constant.language', 'constant.character', 'support.constant'], settings: { foreground: warning } },
		{ scope: ['entity.name.function', 'support.function', 'meta.function-call'], settings: { foreground: blue } },
		{
			scope: ['entity.name.type', 'entity.name.class', 'support.type', 'support.class', 'entity.other.inherited-class'],
			settings: { foreground: info },
		},
		{ scope: ['variable', 'meta.definition.variable', 'support.variable'], settings: { foreground: text } },
		{ scope: ['variable.parameter', 'meta.parameter'], settings: { foreground: textSecondary } },
		{ scope: ['entity.name.tag', 'punctuation.definition.tag'], settings: { foreground: magenta } },
		{ scope: ['entity.other.attribute-name'], settings: { foreground: info } },
		{ scope: ['invalid', 'invalid.illegal'], settings: { foreground: error } },

		{ scope: ['markup.heading', 'entity.name.section'], settings: { foreground: accent, fontStyle: 'bold' } },
		{ scope: ['markup.bold'], settings: { fontStyle: 'bold' } },
		{ scope: ['markup.italic'], settings: { fontStyle: 'italic' } },
		{ scope: ['markup.underline.link', 'string.other.link'], settings: { foreground: info } },
		{ scope: ['markup.inserted', 'meta.diff.header.to-file'], settings: { foreground: accent } },
		{ scope: ['markup.deleted', 'meta.diff.header.from-file'], settings: { foreground: error } },
		{ scope: ['markup.changed'], settings: { foreground: warning } },
		{ scope: ['meta.diff.range', 'punctuation.definition.range.diff'], settings: { foreground: info } },

		{ scope: ['source.shell', 'source.bash'], settings: { foreground: text } },
		{ scope: ['entity.name.command', 'support.function.builtin.shell'], settings: { foreground: blue } },
		{ scope: ['constant.other.option', 'string.unquoted.argument'], settings: { foreground: textSecondary } },
	],
};

export default cipherCodeTheme;
