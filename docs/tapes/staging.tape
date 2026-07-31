# revision — staging a file and a whole directory.
# Embedded on the Staging & commit page.

Output site/public/demos/staging.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1500ms
Show

Sleep 1200ms

# A single file: space stages it, and space again takes it back out.
Type "g"
Sleep 700ms
Type@250ms "jj"
Sleep 1400ms
Space
Sleep 2200ms
Space
Sleep 2200ms

# A directory row acts on everything beneath it in one keystroke.
Type@250ms "jj"
Sleep 1600ms
Space
Sleep 2800ms

# The root row covers the whole tree — the untracked files are svn added first.
Type "g"
Sleep 1200ms
Space
Sleep 3s
