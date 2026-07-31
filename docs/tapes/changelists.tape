# revision — naming a changelist, then reading and drilling into it.
# Embedded on the Changelists page.

Output site/public/demos/changelists.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1500ms
Show

Sleep 1000ms

# Stage the server directory, then n moves the whole staged set into a named
# changelist in one step.
Type "g"
Sleep 700ms
Type@250ms "jjjj"
Sleep 1400ms
Space
Sleep 2s
Type "n"
Sleep 1400ms
Type "feature-health"
Sleep 1400ms
Enter
Sleep 2500ms

# ] turns the Files panel to the Changelists view: each group with its file
# count, and Main showing the whole list as one patch.
Type "]"
Sleep 3s

# enter drills into a list — the panel border takes its name.
Enter
Sleep 3s

# esc goes back to the overview.
Escape
Sleep 2s
