# revision — hero demo tape
#
# Generates docs/hero.gif with VHS (https://github.com/charmbracelet/vhs).
# Run from the repo root:
#
#     vhs docs/demo.tape
#
# Requires: vhs, svn + svnadmin on PATH, and a Go toolchain (to build the
# binary). The hidden setup block builds a throwaway SVN working copy so the
# TUI has real status, diffs, changelists, and history to show.

Output docs/hero.gif

Set Shell "bash"
Set FontSize 16
Set Width 1240
Set Height 760
Set Padding 18
Set Margin 26
Set MarginFill "#16161e"
Set BorderRadius 10
Set WindowBar Colorful
Set TypingSpeed 55ms
Set Framerate 24
Set Theme { "name": "Tokyo Night", "background": "#1a1b26", "foreground": "#c0caf5", "black": "#15161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#414868", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5", "selection": "#283457", "cursor": "#c0caf5" }

# --- hidden setup: build the demo working copy and put revision on PATH ---
Hide
Type "bash docs/demo-setup.sh /tmp/revision-demo && go build -o /tmp/revision-demo/revision ./cmd/revision && echo READY" Enter
Sleep 9s
Type 'export PATH="/tmp/revision-demo:$PATH"' Enter
Type "cd /tmp/revision-demo/wc && clear" Enter
Show

# --- launch: the lazygit-style layout (Status · Files · Log · Main) ---------
# The changed files are shown as a collapsible directory tree rooted at "/".
Type "revision" Enter
Sleep 2.8s

# The Main panel follows the selection with a colour-coded diff. Focus it and
# jump to the end of a long line: the +/- gutter stays pinned and the
# scrollbars show how much is off-screen.
Type "0"
Sleep 1.1s
Type "$"
Sleep 1.9s
Type "^"
Sleep 1s
Type "2"
Sleep 800ms

# Walk down into a directory; highlighting it shows the combined diff of every
# change beneath it.
Type "j"
Sleep 1.5s
Type "j"
Sleep 2s

# Stage the whole directory with a single keystroke — every file under it gets
# a green staged marker.
Space
Sleep 2.2s

# Name the entire staged set as one changelist in a single step.
Type "n"
Sleep 1s
Type "feature-health"
Sleep 1.2s
Enter
Sleep 2.2s

# --- the changelist lives in a tabbed view inside the Files panel -----------
Type "]"
Sleep 2s

# Drill into it to see just its files.
Enter
Sleep 2.2s

# --- read-only history: full revision numbers and authors ------------------
Type "3"
Sleep 1.6s
Type "j"
Sleep 1.6s

# --- the full keybindings menu is always one '?' away ----------------------
Type "?"
Sleep 2.8s
Type "?"
Sleep 800ms

# --- commit the changelist through the message editor ----------------------
Type "2"
Sleep 800ms
Type "c"
Sleep 1.3s
Type "Add /healthz route and handler"
Sleep 1.4s
Ctrl+S
Sleep 2.4s

# Committed — the toast reports the new revision and the log picks it up.
Sleep 1.2s
Type "q"
Sleep 1s
