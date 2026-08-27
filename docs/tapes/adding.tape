# revision — putting untracked files under version control with `a`.
# Embedded on the Staging & commit page.

Output site/public/demos/adding.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1500ms
Show

Sleep 1200ms

# G lands on the last row, which is the untracked notes.txt. `a` schedules it:
# the row restyles from ? to A on the keypress, without a prompt — adding is not
# destructive.
Type "G"
Sleep 1600ms
Type "a"
Sleep 3s

# r undoes it. Reverting an add un-schedules the file and leaves it on disk,
# untracked again — and, being destructive, it confirms first.
Type "r"
Sleep 1800ms
Type "y"
Sleep 3s

# On a directory row `a` takes everything untracked beneath it, so the root row
# adds the whole working copy's worth in one keystroke and one svn invocation.
Type "g"
Sleep 1600ms
Type "a"
Sleep 3s
