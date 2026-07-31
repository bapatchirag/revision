#!/usr/bin/env bash
# demo-setup.sh — build a throwaway SVN working copy for the revision demo GIF.
#
# Creates a self-contained repository + working copy with a short commit history
# and a realistic set of uncommitted changes spread across a nested source tree
# (modified, added, and untracked files) so the TUI has a directory tree, colour
# diffs, changelists, and history to show.
#
# Usage: docs/demo-setup.sh [dir]   (default: /tmp/revision-demo)
set -euo pipefail

DEMO="${1:-/tmp/revision-demo}"
REPO="$DEMO/repo"
WC="$DEMO/wc"
AUTHOR="alice"

rm -rf "$DEMO"
mkdir -p "$DEMO"

svnadmin create "$REPO"

# Commits against a file:// repository are authored by whoever runs this script,
# and the recordings made from it are published. A post-commit hook rewrites the
# author of every revision to a fixed name, including the commits made on camera
# during a recording. Hooks run with an empty environment, so svnadmin is baked
# in by absolute path.
printf '%s' "$AUTHOR" > "$REPO/hooks/author.txt"
printf '#!/bin/sh\nexit 0\n' > "$REPO/hooks/pre-revprop-change"
cat > "$REPO/hooks/post-commit" <<EOF
#!/bin/sh
exec $(command -v svnadmin) setrevprop "\$1" -r "\$2" svn:author "\$1/hooks/author.txt"
EOF
chmod +x "$REPO/hooks/pre-revprop-change" "$REPO/hooks/post-commit"

svn checkout "file://$REPO" "$WC" -q
cd "$WC"

# --- r1: project skeleton -------------------------------------------------
mkdir -p cmd
cat > go.mod <<'EOF'
module orbit

go 1.22
EOF
cat > README.md <<'EOF'
# orbit

A tiny HTTP service.
EOF
cat > cmd/main.go <<'EOF'
package main

import "log"

func main() {
	log.Println("starting orbit")
}
EOF
svn add -q go.mod README.md cmd
svn commit -q -m "Initial import: project skeleton"

# --- r2: HTTP server ------------------------------------------------------
mkdir -p internal/server
cat > internal/server/server.go <<'EOF'
package server

import "net/http"

func New(addr string) *http.Server {
	mux := http.NewServeMux()
	return &http.Server{Addr: addr, Handler: mux}
}
EOF
cat > cmd/main.go <<'EOF'
package main

import (
	"log"

	"orbit/internal/server"
)

func main() {
	log.Println("starting orbit")
	srv := server.New(":8080")
	log.Fatal(srv.ListenAndServe())
}
EOF
svn add -q internal
svn commit -q -m "Add HTTP server skeleton"

# --- r3: configuration ----------------------------------------------------
mkdir -p internal/config
cat > internal/config/config.go <<'EOF'
package config

// Config holds runtime settings for the service.
type Config struct {
	Addr     string
	LogLevel string
}

// Load reads configuration from the given path.
func Load(path string) *Config {
	return &Config{Addr: ":8080", LogLevel: "info"}
}
EOF
cat > cmd/main.go <<'EOF'
package main

import (
	"log"

	"orbit/internal/config"
	"orbit/internal/server"
)

func main() {
	cfg := config.Load("config.yaml")
	log.Printf("starting orbit on %s", cfg.Addr)
	srv := server.New(cfg.Addr)
	log.Fatal(srv.ListenAndServe())
}
EOF
svn add -q internal/config
svn commit -q -m "Load configuration from YAML"
svn update -q

# --- r4: committed from a second checkout ---------------------------------
# $WC deliberately stays at r3. Being behind HEAD is the normal state of a
# working copy, it gives the Status panel a HEAD to report against, and it gives
# the update demo somewhere to go. The revision touches only README.md, which has
# no local modifications, so it merges cleanly in either direction.
svn checkout "file://$REPO" "$DEMO/upstream" -q
cat > "$DEMO/upstream/README.md" <<'EOF'
# orbit

A tiny HTTP service.

## Running

    go run ./cmd
EOF
svn commit -q -m "Document how to run the service" "$DEMO/upstream"

# --- uncommitted working-copy changes ------------------------------------
# Modified: cmd/main.go — graceful shutdown, with one deliberately long line so
# the diff overflows the Main panel and the horizontal scrollbar + pinned gutter
# have something to show.
cat > cmd/main.go <<'EOF'
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"orbit/internal/config"
	"orbit/internal/server"
)

func main() {
	cfg := config.Load("config.yaml")
	log.Printf("starting orbit on %s with log level %s and graceful shutdown via SIGINT/SIGTERM", cfg.Addr, cfg.LogLevel)

	srv := server.New(cfg.Addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down orbit")
}
EOF

# Modified: internal/server/server.go — register a health route.
cat > internal/server/server.go <<'EOF'
package server

import "net/http"

func New(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealth)
	return &http.Server{Addr: addr, Handler: mux}
}
EOF

# Added: internal/server/handler.go — a new, svn-added file (unassigned, so the
# demo can stage the whole internal/server directory at once).
cat > internal/server/handler.go <<'EOF'
package server

import "net/http"

// handleHealth reports service liveness for readiness probes.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
EOF
svn add -q internal/server/handler.go

# Untracked scratch files at the working-copy root.
printf 'ORBIT_ADDR=:9090\nORBIT_LOG_LEVEL=debug\n' > .env
printf 'TODO: graceful shutdown timeout\nTODO: structured logging\n' > notes.txt

echo "demo ready: $WC"
