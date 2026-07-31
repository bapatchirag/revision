# Fonts

Self-hosted, subset web fonts. Both are SIL OFL 1.1; the licence files ship
alongside and must stay next to the `.woff2` files.

| File | Upstream | Licence |
|---|---|---|
| `FantasqueSansMono-{Regular,Bold,Italic}.subset.woff2` | [belluzj/fantasque-sans](https://github.com/belluzj/fantasque-sans) v1.8.0, unpatched `TTF/` build | `FantasqueSansMono-OFL.txt` |
| `JetBrainsMono-Regular.fallback.woff2` | [JetBrains/JetBrainsMono](https://github.com/JetBrains/JetBrainsMono) v2.304 | `JetBrainsMono-OFL.txt` |

Never use the Nerd Font patched build of Fantasque — it carries megabytes of
icon glyphs that this site does not use.

## Why two faces

Fantasque covers Box Drawing (U+2500–257F) and Block Elements (U+2580–259F)
completely, so TUI frames render in a single face at a single cell width. It is
missing only a handful of glyphs the app emits — `▾`, `◆`, plus `✓` / `✗` and
arrows beyond U+2190–2193 — which JetBrains Mono supplies.

The two have different advance ratios (Fantasque 1060/2048 = 0.51758em,
JetBrains 600/1000 = 0.6em), so `src/styles/fonts.css` applies
`size-adjust: 86.263%` to the fallback. Without it, every fallback glyph would
be 16% too wide and break vertical alignment in box-drawn output.

## Regenerating

Requires `fonttools` and `brotli`.

```sh
FQ='U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+2000-206F,U+2074,U+20AC,U+2122,U+2190-2193,U+2212,U+2500-259F,U+25A0-25FF,U+FEFF,U+FFFD'

for w in Regular Bold Italic; do
  pyftsubset "FantasqueSansMono-$w.ttf" \
    --output-file="FantasqueSansMono-$w.subset.woff2" \
    --flavor=woff2 --unicodes="$FQ" \
    --layout-features='kern,liga,calt,clig,ccmp,rlig,mark,mkmk' \
    --name-IDs='*' --name-legacy --notdef-outline --no-hinting
done

pyftsubset JetBrainsMono-Regular.ttf \
  --output-file=JetBrainsMono-Regular.fallback.woff2 \
  --flavor=woff2 --unicodes='U+2190-21FF,U+25A0-25FF,U+2713,U+2717' \
  --layout-features='' --name-IDs='*' --name-legacy --notdef-outline --no-hinting
```

If the Fantasque advance ratio changes upstream, recompute `size-adjust` as
`fantasqueAdvance / fantasqueUpm / (jetbrainsAdvance / jetbrainsUpm)`.
