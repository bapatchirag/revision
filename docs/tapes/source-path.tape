# revision — moving the source path to a subdirectory for the session.
# Embedded on the Panels page, beside the Status panel's Source row.

Output site/public/demos/source-path.mp4
Source docs/tapes/common.tape

Hide
Type "1"
Sleep 1500ms
Show

Sleep 1800ms

# P opens on the directory revision is reading now and lists what is under it.
# The working copy's root is fixed at the head of the input, so the source can
# move anywhere inside the checkout but never out of it.
Type "P"
Sleep 2500ms

# Typing re-lists as it goes, so the prompt doubles as a browser.
Type "internal/"
Sleep 2500ms

# Tab moves into the suggestions; the highlighted one is written into the input.
# The list is led by the parent directory, so it walks back up as well as down.
Tab
Sleep 1200ms
Down
Sleep 900ms
Down
Sleep 1500ms
Tab
Sleep 1200ms
Enter
Sleep 3s

# Status now names the new source, and the Files tree is scoped to it.
Type "2"
Sleep 3500ms
