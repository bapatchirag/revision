# revision — hero demo tape
#
# Generates the demo recordings with VHS (https://github.com/charmbracelet/vhs).
# Run from the repo root:
#
#     vhs docs/demo.tape
#
# Requires: vhs, svn + svnadmin on PATH, and a Go toolchain (to build the
# binary). The hidden setup block builds a throwaway SVN working copy so the
# TUI has real status, diffs, changelists, saved patches, a reject and history
# to show, and points XDG_CONFIG_HOME at the same throwaway directory so the
# settings demo never touches the real ~/.config/revision/config.json.
#
# Unlike the per-feature tapes in docs/tapes, this one keeps a window bar and a
# margin — it is the poster shot. The palette is Cipher either way: once for the
# terminal VHS renders into, once for revision itself via docs/tapes/config.json.
#
# The width is not arbitrary. With all four views named, the Files panel's tab
# strip is 53 cells and the left column is 2/5 of the terminal, so under about
# 140 columns "Rejects" is truncated off the border.

Output site/public/demos/hero.gif

Set Shell "bash"
Set FontSize 16
Set Width 1560
Set Height 940
Set Padding 18
Set Margin 26
Set MarginFill "#111927"
Set BorderRadius 10
Set WindowBar Colorful
Set TypingSpeed 55ms
Set Framerate 20
Set WaitTimeout 3m
Set Theme { "name": "Cipher", "background": "#111927", "foreground": "#e6e9f2", "black": "#141d2b", "red": "#ff3e3e", "green": "#9fef00", "yellow": "#ffaf00", "blue": "#5cb2ff", "magenta": "#c88dea", "cyan": "#4dd2e1", "white": "#a4b1cd", "brightBlack": "#7a7899", "brightRed": "#ff8484", "brightGreen": "#c5f467", "brightYellow": "#ffcc5c", "brightBlue": "#5cb2ff", "brightMagenta": "#c88dea", "brightCyan": "#4dd2e1", "brightWhite": "#ffffff", "selection": "#302540", "cursor": "#9fef00" }

# --- hidden setup: build the demo working copy and put revision on PATH ---
Hide

# A bare prompt: nothing is recorded here, but a default PS1 carries the
# hostname and the checkout path, and one mistimed frame would publish both.
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
Show

# --- launch: the lazygit-style layout ---------------------------------------
# Status, Files and Log on the left, the Main detail view on the right, and the
# command log of every svn action underneath it.
Type "revision" Enter
Wait+Screen /Command Log/
Sleep 2.5s

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

# --- the Files panel: Changes -----------------------------------------------
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

# w writes whatever diff is on screen out as a patch file.
Type "w"
Sleep 1.1s
Type "server-health"
Sleep 1.1s
Enter
Sleep 2s

# Name the entire staged set as one changelist in a single step.
Type "n"
Sleep 900ms
Type "feature-health"
Sleep 1.1s
Enter
Sleep 1.9s

# --- the Files panel holds four views, cycled with [ and ] -------------------
# Changelists: the staged set, grouped under the name it was just given.
Type "]"
Sleep 1.8s

# Drill into one to see only its files; esc comes back out.
Enter
Sleep 1.8s
Escape
Sleep 1.2s

# Diffs: the patches saved with w, the highlighted one rendered in Main.
Type "]"
Sleep 2.4s

# Rejects: the hunks a patch could not place, found by walking the working copy.
# svn ignores .rej files, so the Changes view never lists them.
Type "]"
Sleep 2.6s

# Back round to Changes.
Type "]"
Sleep 1.2s

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
