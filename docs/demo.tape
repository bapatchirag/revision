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
Sleep 8s
Type 'export PATH="/tmp/revision-demo:$PATH"' Enter
Type "cd /tmp/revision-demo/wc && clear" Enter
Show

# --- launch: the lazygit-style layout (Status · Files · Log · Main) ---------
Type "revision" Enter
Sleep 2.5s

# Browse the changed files; the Main panel follows the selection with a diff.
Type "j"
Sleep 1.6s
Type "j"
Sleep 1.8s

# Stage a modified file — a green dot appears and a toast confirms it.
Space
Sleep 2s

# Move to another change and stage it too.
Type "j"
Sleep 700ms
Type "j"
Sleep 1.6s
Space
Sleep 2s

# --- named changelists live in a tabbed view inside the Files panel --------
Type "]"
Sleep 2.2s

# Drill into a changelist to see just its files, then step back out.
Enter
Sleep 2.2s
Escape
Sleep 1.2s
Type "["
Sleep 1.2s

# --- read-only history in the Log panel ------------------------------------
Type "3"
Sleep 1.6s
Type "j"
Sleep 1.6s

# --- the full keybindings menu is always one '?' away ----------------------
Type "?"
Sleep 3.2s
Type "?"
Sleep 900ms

# --- commit the staged set through the message editor ----------------------
Type "2"
Sleep 900ms
Type "c"
Sleep 1.4s
Type "Wire graceful shutdown and /healthz route"
Sleep 1.6s
Ctrl+S
Sleep 2.6s

# Committed — the toast reports the new revision and the log picks it up.
Sleep 1.4s
Type "q"
Sleep 1s
