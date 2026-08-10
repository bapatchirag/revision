#!/usr/bin/env bash
# resolve-setup.sh — build a throwaway SVN working copy that needs resolving,
# for testing the `m` overlay by hand.
#
# Leaves the working copy in the three states the overlay is for:
#   * internal/greet.go   — conflicted, with TWO regions, so [ and ] page
#                           between them and the pane labels re-head each time;
#   * cmd/main.go         — conflicted, with a single region;
#   * internal/handlers.go.svnpatch.rej — a hunk `svn patch` would not place,
#                           left behind after the file was put back, so its text
#                           is findable again and the Rejects view has one to
#                           decide rather than to report as unplaceable.
#
# Usage: docs/resolve-setup.sh [dir]   (default: /tmp/revision-resolve)
#        revision -path /tmp/revision-resolve/wc
set -euo pipefail

DEMO="${1:-/tmp/revision-resolve}"
REPO="$DEMO/repo"
WC="$DEMO/wc"
UPSTREAM="$DEMO/upstream"

rm -rf "$DEMO"
mkdir -p "$DEMO"

svnadmin create "$REPO"
svn checkout "file://$REPO" "$WC" -q
cd "$WC"
mkdir -p cmd internal

# --- r1: the code everything below disagrees about ------------------------
# greet.go's two greetings are kept well apart: diff3 runs neighbouring
# differences together into one block, and two regions are the point here.
cat > internal/greet.go <<'EOF'
package internal

import "fmt"

// Greet writes a greeting for name.
func Greet(name string) {
	fmt.Printf("hello, %s\n", name)
}

// Sep is the rule printed between a greeting and a parting line. It also keeps
// the two apart, so a conflict in one does not swallow the other.
func Sep() {
	fmt.Println("----------")
	fmt.Println("")
	fmt.Println("----------")
}

// Farewell writes a parting line for name.
func Farewell(name string) {
	fmt.Printf("bye, %s\n", name)
}
EOF

cat > cmd/main.go <<'EOF'
package main

import "log"

func main() {
	log.Println("starting orbit on :8080")
}
EOF

cat > internal/handlers.go <<'EOF'
package internal

import "net/http"

func handleUsers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("users"))
}

// Routes wires the handlers onto a mux. It sits between the two handlers so a
// change to each lands in a hunk of its own.
func Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/users", handleUsers)
	mux.HandleFunc("/orders", handleOrders)
	mux.HandleFunc("/healthz", handleHealth)
	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleOrders(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("orders"))
}
EOF

svn add -q cmd internal
svn commit -q -m "Initial import"
svn update -q

# --- a reject left behind by a patch that half went in ---------------------
# The patch touches both handlers, far enough apart to be two hunks.
sed -i '' 's|"users"|"users v2"|; s|"orders"|"orders v2"|' internal/handlers.go
svn diff internal/handlers.go > "$DEMO/handlers.patch"
svn revert -q internal/handlers.go

# svn looks for a hunk's text anywhere in the target, so a hunk is only rejected
# once that text is gone. handleOrders is edited out of the way, the patch is
# applied — the users hunk goes in, the orders hunk is written to a .rej — and
# then the file is put back the way it was. The reject outlives the file it
# failed against, which is why its hunk can be placed again: that outward search
# is the whole point of the Rejects half of the overlay.
sed -i '' 's|_, _ = w.Write(\[\]byte("orders"))|_, _ = w.Write([]byte("orders (rewritten)"))|' \
	internal/handlers.go
svn patch "$DEMO/handlers.patch" > /dev/null
svn revert -q internal/handlers.go

# --- r2: what someone else committed --------------------------------------
svn checkout "file://$REPO" "$UPSTREAM" -q
sed -i '' 's|"hello, %s\\n"|"Hello, %s!\\n"|; s|"bye, %s\\n"|"Goodbye, %s!\\n"|' \
	"$UPSTREAM/internal/greet.go"
sed -i '' 's|starting orbit on :8080|listening on :8080|' "$UPSTREAM/cmd/main.go"
svn commit -q -m "Punctuate the greetings" "$UPSTREAM"

# --- the local changes that collide with it -------------------------------
sed -i '' 's|"hello, %s\\n"|"hi there, %s\\n"|; s|"bye, %s\\n"|"see you, %s\\n"|' internal/greet.go
sed -i '' 's|starting orbit on :8080|serving HTTP on :8080|' cmd/main.go

# --accept postpone leaves the markers in the file rather than asking which side
# to take, which is the state the overlay reads.
svn update --accept postpone --non-interactive -q

printf '\n%s\n' "--- svn status ---"
svn status
printf '\n%s\n' "--- rejects ---"
find . -name '*.rej' -not -path './.svn/*'
printf '\n%s\n' "--- conflict markers ---"
grep -c '<<<<<<<' internal/greet.go cmd/main.go
printf '\n%s\n' "Run it with:  revision -path $WC"
