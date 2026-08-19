# revision — picking two revisions, diffing them, and browsing what they touched.
# Embedded on the Working with diffs page.

Output site/public/demos/revision-diff.mp4
Source docs/tapes/common.tape

Hide
Type "3"
Sleep 1500ms
Type "g"
Sleep 800ms
Show

Sleep 1200ms

# v picks the highlighted revision to be compared. A dot marks each pick and the
# hint bar names what is held; two can be held at once.
Type@250ms "jj"
Sleep 1200ms
Type "v"
Sleep 1800ms
Type@250ms "kk"
Sleep 1000ms
Type "v"
Sleep 2200ms

# enter diffs them — always oldest to newest — and the panel becomes the tree of
# files the range touched, titled with the range.
Enter
Sleep 3s

# Main follows the cursor exactly as it does in the Changes view: a file shows
# its own section of the patch…
Type@300ms "jj"
Sleep 2200ms

# …and a directory everything beneath it.
Type "j"
Sleep 2200ms
Type@300ms "jjj"
Sleep 2500ms

# esc comes back out to the revisions, with the picks still held.
Escape
Sleep 2200ms
