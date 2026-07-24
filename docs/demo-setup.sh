#!/usr/bin/env bash
# demo-setup.sh — build a throwaway SVN working copy for the revision demo GIF.
#
# Creates a self-contained repository + working copy with a short commit history
# and a realistic set of uncommitted changes (modified, added, untracked, and a
# named changelist) so the TUI has something interesting to show.
#
# Usage: docs/demo-setup.sh [dir]   (default: /tmp/revision-demo)
set -euo pipefail

DEMO="${1:-/tmp/revision-demo}"
REPO="$DEMO/repo"
WC="$DEMO/wc"

rm -rf "$DEMO"
mkdir -p "$DEMO"

svnadmin create "$REPO"
svn checkout "file://$REPO" "$WC" -q
cd "$WC"

# --- r1: project skeleton -------------------------------------------------
cat > README.md <<'EOF'
# orbit

A tiny HTTP service.
EOF
cat > main.go <<'EOF'
package main

import "log"

func main() {
	log.Println("starting orbit")
}
EOF
svn add -q README.md main.go
svn commit -q -m "Initial import: project skeleton"

# --- r2: HTTP server ------------------------------------------------------
cat > server.go <<'EOF'
package main

import "net/http"

func newServer(addr string) *http.Server {
	mux := http.NewServeMux()
	return &http.Server{Addr: addr, Handler: mux}
}
EOF
cat > main.go <<'EOF'
package main

import "log"

func main() {
	log.Println("starting orbit")
	srv := newServer(":8080")
	log.Fatal(srv.ListenAndServe())
}
EOF
svn add -q server.go
svn commit -q -m "Add HTTP server skeleton"

# --- r3: configuration ----------------------------------------------------
cat > config.yaml <<'EOF'
addr: ":8080"
log_level: "info"
EOF
cat > main.go <<'EOF'
package main

import "log"

func main() {
	cfg := loadConfig("config.yaml")
	log.Printf("starting orbit on %s", cfg.Addr)
	srv := newServer(cfg.Addr)
	log.Fatal(srv.ListenAndServe())
}
EOF
svn add -q config.yaml
svn commit -q -m "Load configuration from YAML"
svn update -q

# --- uncommitted working-copy changes ------------------------------------
# Modified: main.go (add graceful shutdown) and server.go (register a route).
cat > main.go <<'EOF'
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
)

func main() {
	cfg := loadConfig("config.yaml")
	log.Printf("starting orbit on %s", cfg.Addr)

	srv := newServer(cfg.Addr)
	go func() { log.Fatal(srv.ListenAndServe()) }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
}
EOF
cat > server.go <<'EOF'
package main

import (
	"net/http"
)

func newServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealth)
	return &http.Server{Addr: addr, Handler: mux}
}
EOF

# Added + staged into a named changelist.
cat > handler.go <<'EOF'
package main

import "net/http"

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
EOF
svn add -q handler.go
svn changelist feature-health handler.go -q

# Untracked scratch files.
printf 'ORBIT_ADDR=:9090\nORBIT_LOG_LEVEL=debug\n' > .env
printf 'TODO: add graceful shutdown timeout\nTODO: structured logging\n' > notes.txt

echo "demo ready: $WC"
