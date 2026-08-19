# revision — keeping files out of the Changes view for good with a hide rule.
# Embedded on the Configuration page, beside hideRules.

Output site/public/demos/hide-rules.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1500ms
Show

Sleep 1500ms

# The two scratch files at the root are always in the way. S opens the settings,
# and the Hide rules row opens an editor of its own.
Type "S"
Sleep 1500ms
Down@140ms 11
Sleep 1400ms
Enter
Sleep 2s

# a adds a rule: a Go regular expression matched against the path, unanchored.
Type "a"
Sleep 1200ms
Type@90ms "\.env$"
Sleep 1500ms
Enter
Sleep 1800ms
Type "a"
Sleep 900ms
Type@90ms "^notes"
Sleep 1500ms
Enter
Sleep 2s

# space parks a rule without deleting it, so one can be kept as written and
# turned back on later.
Space
Sleep 1800ms
Space
Sleep 1600ms

# ctrl+s keeps the rules; ctrl+s again saves the settings, and the rules take
# effect as the form closes.
Ctrl+S
Sleep 1500ms
Ctrl+S
Sleep 2500ms

# Both files have left the tree. Nothing is ignored or excluded — the footer
# still counts them in brackets.
Type "2"
Sleep 3s
