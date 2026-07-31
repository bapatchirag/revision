# revision — reading a diff side by side.
# Embedded on the Working with diffs page.

Output site/public/demos/split-diff.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1200ms
Type "g"
Sleep 1s
Show

# The root row is the whole working copy as one patch; s reopens it as a
# two-pane comparison in a centered overlay.
Sleep 1500ms
Type "s"
Sleep 3s

# Both panes scroll together.
Down@70ms 14
Sleep 2s

# A multi-file diff is paged one file at a time; the bottom border names the
# open file, its line range and the page number.
Type "]"
Sleep 2500ms
Type "]"
Sleep 2500ms
Type "["
Sleep 2s

# s or esc closes it.
Escape
Sleep 1800ms
