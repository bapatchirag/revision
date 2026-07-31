# revision — writing a diff to a file, then finding it in the Diffs view.
# Embedded on the Working with diffs page.

Output site/public/demos/save-diff.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1200ms
Type "g"
Sleep 800ms
Type@250ms "jjjj"
Sleep 1200ms
Show

Sleep 1500ms

# w writes the diff on screen to a file. The prompt suggests a name; leaving it
# blank uses the suggestion.
Type "w"
Sleep 2500ms
Enter
Sleep 3s

# ] twice opens the Diffs view — the patches already in diffOutputDir, newest
# first — and selecting one shows it in Main.
Type "]"
Sleep 1500ms
Type "]"
Sleep 3500ms

# [ turns back to the Changes view.
Type "["
Sleep 1200ms
Type "["
Sleep 1800ms
