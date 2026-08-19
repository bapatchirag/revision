# revision — picking changes, shelving them, and popping them back.
# Embedded on the Shelving page.

Output site/public/demos/shelving.mp4
Source docs/tapes/common.tape

Hide
Type "2"
Sleep 1500ms
Show

Sleep 1200ms

# v holds a file for the next shelve; the tick and the count follow it.
Type "g"
Sleep 700ms
Type@250ms "jj"
Sleep 1200ms
Type "v"
Sleep 1800ms
Type "j"
Sleep 800ms
Type "v"
Sleep 2200ms

# z takes what is held; the prompt names the entry.
Type "z"
Sleep 1800ms
Type@90ms "wip refactor"
Sleep 1200ms
Enter
Sleep 3s

# The Changes view has emptied and the entry is on the shelf.
Type "4"
Sleep 2500ms

# Selecting it reads its patch into Main.
Sleep 2s

# p pops it: the changes come back and the entry leaves the shelf.
Type "p"
Sleep 2200ms
Enter
Sleep 3500ms

Type "2"
Sleep 3s
