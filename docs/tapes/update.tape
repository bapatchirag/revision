# revision — moving the working copy to an older revision, then back to HEAD.
# Embedded on the Updating the working copy page.

Output site/public/demos/update.mp4
Source docs/tapes/common.tape

Hide
Type "1"
Sleep 1500ms
Show

Sleep 1800ms

# The working copy sits at r3 while the repository is at r4. u catches it up —
# always behind a confirmation.
Type "u"
Sleep 2s
Enter
Sleep 4s

# space in the Log moves the working copy to any revision, in either direction.
# The asterisk marks where it currently sits.
Type "3"
Sleep 1500ms
Type "g"
Sleep 600ms
Type "j"
Sleep 1600ms
Space
Sleep 2500ms
Enter
Sleep 4s

# Both invocations are recorded in the Command Log, marked with their result.
Type "4"
Sleep 3s
