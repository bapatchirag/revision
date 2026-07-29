# revision — hero demo tape
#
# Generates docs/hero.gif with VHS (https://github.com/charmbracelet/vhs).
# Run from the repo root:
#
#     vhs docs/demo.tape
#
# Requires: vhs, svn + svnadmin on PATH, and a Go toolchain (to build the
# binary). The hidden setup block builds a throwaway SVN working copy so the
# TUI has real status, diffs, changelists, and history to show, and points
# XDG_CONFIG_HOME at the same throwaway directory so the settings demo never
# touches the real ~/.config/revision/config.json.

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
Set Framerate 20
Set Theme { "name": "Tokyo Night", "background": "#1a1b26", "foreground": "#c0caf5", "black": "#15161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#414868", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5", "selection": "#283457", "cursor": "#c0caf5" }

# --- hidden setup: build the demo working copy and put revision on PATH ---
Hide
Type "bash docs/demo-setup.sh /tmp/revision-demo && go build -o /tmp/revision-demo/revision ./cmd/revision && echo READY" Enter
Sleep 9s
Type 'export PATH="/tmp/revision-demo:$PATH"' Enter
Type 'export XDG_CONFIG_HOME=/tmp/revision-demo/config' Enter
Type "cd /tmp/revision-demo/wc && clear" Enter
Show

# --- launch: the lazygit-style layout ---------------------------------------
# Status, Files and Log on the left, the Main detail view on the right, and the
# command log of every svn action underneath it.
Type "revision" Enter
Sleep 3s

# The Status panel names the working copy: its root, the branch it came from,
# and the revision it sits at against HEAD.
Type "1"
Sleep 2.4s

# --- the diff follows the selection ----------------------------------------
Type "2"
Sleep 800ms

# Focus Main and scroll it: down into the body, then sideways along a long
# line. The +/- gutter stays pinned while the text slides, and the scrollbars
# show how much is off-screen.
Type "0"
Sleep 800ms
Down@40ms 8
Sleep 800ms
Right@40ms 16
Sleep 1.3s
Type "^"
Sleep 500ms
Type "g"
Sleep 600ms

# '/' searches a detail panel in place: matches stay where they are, lit up,
# and n jumps to the next one.
Type "/"
Sleep 600ms
Type "cfg"
Sleep 1.2s
Enter
Sleep 1.4s
Type "n"
Sleep 1.4s
Escape
Sleep 700ms

# --- the Files panel --------------------------------------------------------
Type "2"
Sleep 700ms

# U hides untracked files; the footer keeps count of what is on screen and how
# much is hidden.
Type "U"
Sleep 1.5s
Type "U"
Sleep 800ms

# On a list panel '/' filters instead — here, down to the modified files.
Type "/"
Sleep 600ms
Type "state:M"
Sleep 1.1s
Enter
Sleep 1.7s
Escape
Sleep 800ms

# Walk back down the tree; highlighting a directory shows the combined diff of
# every change beneath it.
Type "g"
Sleep 600ms
Type@250ms "jjj"
Sleep 1.4s
Type "j"
Sleep 1.8s

# Stage the whole directory with a single keystroke — every file under it gets
# a green staged marker.
Space
Sleep 2s

# Name the entire staged set as one changelist in a single step.
Type "n"
Sleep 900ms
Type "feature-health"
Sleep 1.1s
Enter
Sleep 1.9s

# --- the changelist lives in a tabbed view inside the Files panel -----------
Type "]"
Sleep 1.6s

# Drill into it to see just its files.
Enter
Sleep 1.8s

# --- history: full revision numbers and authors, working copy marked with * --
Type "3"
Sleep 1.4s
Type "j"
Sleep 1.5s

# space moves the working copy to the selected revision — always behind a
# confirmation, so esc backs out.
Space
Sleep 2s
Escape
Sleep 800ms

# --- every svn action is mirrored in the command log ------------------------
Type "4"
Sleep 2.2s

# --- settings: edit the config file in place, previewing the theme live -----
Type "S"
Sleep 1.2s
Down@250ms 3
Sleep 700ms
Right@850ms 4
Sleep 1.1s
Escape
Sleep 1.1s

# --- the full keybindings menu is always one '?' away ----------------------
Type "?"
Sleep 2.4s
Type "?"
Sleep 700ms

# --- commit the changelist through the message editor ----------------------
Type "2"
Sleep 700ms
Type "c"
Sleep 1.2s
Type "Add /healthz route and handler"
Sleep 1.3s
Ctrl+S
Sleep 2.4s

# Committed — the toast reports the new revision, the log picks it up, and the
# commit joins the command log.
Sleep 1.2s
Type "q"
Sleep 800ms
