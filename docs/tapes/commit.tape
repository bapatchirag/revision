# revision — committing the staged set through the message editor.
# Embedded on the Staging & commit page.

Output site/public/demos/commit.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1500ms
Show

Sleep 1000ms

# Stage the server directory…
Type "g"
Sleep 700ms
Type@250ms "jjjj"
Sleep 1500ms
Space
Sleep 2200ms

# …then c opens the inline message editor. ctrl+s submits.
Type "c"
Sleep 1600ms
Type "Add /healthz route and handler"
Sleep 1600ms
Ctrl+S
Sleep 3s

# The committed files leave the tree and the new revision tops the Log.
Type "3"
Sleep 2500ms

# The svn commit invocation is recorded in the Command Log.
Sleep 3s
