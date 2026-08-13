# revision — shared settings and setup for the per-feature demo tapes.
#
# Sourced by every tape in this directory; it is not runnable on its own. It
# declares no Output and ends with the recording still hidden, so each tape can
# put the app where it needs it before its first visible frame.
#
# The palette is pinned twice over: once for the terminal VHS renders into, and
# once for revision itself via docs/tapes/config.json. Both are Cipher, so a
# recording matches the page it is embedded in.

Set Shell "bash"
Set FontSize 18
Set Width 1600
Set Height 900
Set Padding 14
Set Margin 0
Set TypingSpeed 55ms
Set Framerate 20
Set WaitTimeout 3m
Set Theme { "name": "Cipher", "background": "#111927", "foreground": "#e6e9f2", "black": "#141d2b", "red": "#ff3e3e", "green": "#9fef00", "yellow": "#ffaf00", "blue": "#5cb2ff", "magenta": "#c88dea", "cyan": "#4dd2e1", "white": "#a4b1cd", "brightBlack": "#7a7899", "brightRed": "#ff8484", "brightGreen": "#c5f467", "brightYellow": "#ffcc5c", "brightBlue": "#5cb2ff", "brightMagenta": "#c88dea", "brightCyan": "#4dd2e1", "brightWhite": "#ffffff", "selection": "#302540", "cursor": "#9fef00" }

Hide

# A bare prompt: nothing recorded here, but a default PS1 carries the hostname
# and the checkout path, and one mistimed frame would publish both.
Type 'export PS1="$ "' Enter

# Build the binary outside the demo directory — demo-setup.sh wipes that — then
# build the throwaway repository and working copy. The script's own closing
# "demo ready:" line is the signal that both are done, so no guessed sleep.
Type "mkdir -p /tmp/revision-demo-bin && go build -o /tmp/revision-demo-bin/revision ./cmd/revision && bash docs/demo-setup.sh /tmp/revision-demo" Enter
Wait+Screen /demo ready:/

Type 'export PATH="/tmp/revision-demo-bin:$PATH"' Enter
Type "export XDG_CONFIG_HOME=/tmp/revision-demo/config" Enter
Type "mkdir -p $XDG_CONFIG_HOME/revision && cp docs/tapes/config.json $XDG_CONFIG_HOME/revision/config.json" Enter

# Enter the working copy by its physical path. svn reports the working copy root
# with symlinks resolved, so on macOS — where /tmp is a link to /private/tmp —
# landing here via /tmp would put a different string in the Status panel's Root
# row than in its Source and CWD rows. A no-op on Linux.
Type 'cd /tmp/revision-demo/wc && cd "$(pwd -P)" && clear' Enter

Type "revision" Enter
Wait+Screen /Command Log/
Sleep 2s
