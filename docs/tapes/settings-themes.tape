# revision — previewing every built-in palette live in the settings editor.
# Embedded on the Themes page.

Output site/public/demos/settings-themes.mp4
Source docs/tapes/common.tape

Hide
Type "S"
Sleep 1800ms
Show

Sleep 1500ms

# Move to the Theme field…
Down@450ms 2
Sleep 1500ms

# …and cycle it. Each palette is applied to the whole UI as you land on it, so
# you preview it in place. Six presses come back round to where it started.
Right@1200ms 6
Sleep 2500ms

# esc reverts to what you had; ctrl+s keeps it.
Escape
Sleep 2s
