# revision — searching inside a diff: matches stay in place and n walks them.
# Embedded on the Filtering & searching page.

Output site/public/demos/diff-search.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1200ms
Type "0"
Sleep 1200ms
Show

Sleep 1200ms

# In a detail panel / highlights rather than hides: nothing is removed.
Type "/"
Sleep 1s
Type "cfg"
Sleep 1600ms
Enter
Sleep 2200ms

# n and N jump between matches, wrapping at the ends; the hint bar shows the
# position.
Type "n"
Sleep 2s
Type "n"
Sleep 2s
Type "N"
Sleep 2s

# esc clears the query.
Escape
Sleep 1800ms
