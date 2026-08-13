package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// atTime freezes the package clock for the duration of a test.
func atTime(t *testing.T, at time.Time) {
	t.Helper()
	prev := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = prev })
}

// countingAPI is withAPI with a tally of how many times GitHub was actually
// called, which is the whole point of the memo.
func countingAPI(t *testing.T, tagName string, status int) *int {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"tag_name":"` + tagName + `","html_url":"https://example.test/release"}`))
		}
	}))
	t.Cleanup(srv.Close)

	prev := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = prev })
	return &calls
}

func TestCheckCachedAsksOnceWithinTheInterval(t *testing.T) {
	calls := countingAPI(t, "v1.5.0", http.StatusOK)
	path := filepath.Join(t.TempDir(), "update-check.json")
	build := Build{Version: "1.4.0", Channel: "release"}

	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	atTime(t, start)
	rel, newer, err := CheckCached(context.Background(), build, path)
	if err != nil || !newer || rel.Tag != "v1.5.0" {
		t.Fatalf("first check = %+v/%v/%v, want the new release", rel, newer, err)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d, want the first check to reach GitHub", *calls)
	}

	// A second launch an hour later answers from the memo, and still reports the
	// update, so throttling does not mean forgetting.
	atTime(t, start.Add(time.Hour))
	rel, newer, err = CheckCached(context.Background(), build, path)
	if err != nil || !newer || rel.Tag != "v1.5.0" {
		t.Fatalf("cached check = %+v/%v/%v, want the remembered release", rel, newer, err)
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want the memo to answer without GitHub", *calls)
	}

	// A day later it is worth asking again.
	atTime(t, start.Add(checkInterval))
	if _, _, err := CheckCached(context.Background(), build, path); err != nil {
		t.Fatalf("stale check: %v", err)
	}
	if *calls != 2 {
		t.Errorf("calls = %d, want a stale memo to ask again", *calls)
	}
}

func TestCheckCachedRemembersThatItIsUpToDate(t *testing.T) {
	calls := countingAPI(t, "v1.4.0", http.StatusOK)
	path := filepath.Join(t.TempDir(), "update-check.json")
	build := Build{Version: "1.4.0", Channel: "release"}

	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	atTime(t, start)
	if _, newer, _ := CheckCached(context.Background(), build, path); newer {
		t.Fatal("the running version is the latest")
	}
	atTime(t, start.Add(time.Hour))
	if _, newer, _ := CheckCached(context.Background(), build, path); newer {
		t.Error("the memo must not invent an update")
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want one", *calls)
	}
}

func TestCheckCachedBacksOffAfterAFailure(t *testing.T) {
	calls := countingAPI(t, "", http.StatusInternalServerError)
	path := filepath.Join(t.TempDir(), "update-check.json")
	build := Build{Version: "1.4.0", Channel: "release"}

	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	atTime(t, start)
	if _, _, err := CheckCached(context.Background(), build, path); err == nil {
		t.Fatal("expected the failure to be reported the first time")
	}

	// Launching again straight away must not hammer an API that just refused.
	atTime(t, start.Add(time.Minute))
	if _, newer, err := CheckCached(context.Background(), build, path); err != nil || newer {
		t.Errorf("backed-off check = %v/%v, want a quiet no", newer, err)
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want the failure to hold off the next attempt", *calls)
	}

	// The backoff is shorter than a successful check's interval.
	atTime(t, start.Add(failureBackoff))
	if _, _, err := CheckCached(context.Background(), build, path); err == nil {
		t.Error("expected the check to be retried once the backoff expired")
	}
	if *calls != 2 {
		t.Errorf("calls = %d, want a retry after the backoff", *calls)
	}
}

func TestCheckCachedSurvivesAnUnusableMemo(t *testing.T) {
	calls := countingAPI(t, "v1.5.0", http.StatusOK)
	path := filepath.Join(t.TempDir(), "update-check.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write memo: %v", err)
	}
	atTime(t, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))

	if _, newer, err := CheckCached(context.Background(), Build{Version: "1.4.0", Channel: "release"}, path); err != nil || !newer {
		t.Fatalf("check = %v/%v, want a corrupt memo to be ignored", newer, err)
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want the check to run", *calls)
	}
}

func TestCheckCachedIgnoresAClockThatWentBackwards(t *testing.T) {
	calls := countingAPI(t, "v1.5.0", http.StatusOK)
	path := filepath.Join(t.TempDir(), "update-check.json")
	build := Build{Version: "1.4.0", Channel: "release"}

	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	atTime(t, start)
	if _, _, err := CheckCached(context.Background(), build, path); err != nil {
		t.Fatalf("first check: %v", err)
	}
	// A memo stamped in the future would otherwise never age out.
	atTime(t, start.Add(-48*time.Hour))
	if _, _, err := CheckCached(context.Background(), build, path); err != nil {
		t.Fatalf("check after the clock moved: %v", err)
	}
	if *calls != 2 {
		t.Errorf("calls = %d, want the check to run rather than trust a future memo", *calls)
	}
}

func TestCheckCachedSkipsDevBuildsWithoutTouchingTheMemo(t *testing.T) {
	calls := countingAPI(t, "v1.5.0", http.StatusOK)
	path := filepath.Join(t.TempDir(), "update-check.json")
	atTime(t, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))

	if _, newer, err := CheckCached(context.Background(), Build{Version: "dev", Channel: "dev"}, path); err != nil || newer {
		t.Fatalf("dev build = %v/%v, want a silent no", newer, err)
	}
	if *calls != 0 {
		t.Errorf("calls = %d, want none", *calls)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a development build should leave nothing behind")
	}
}

func TestCheckCachedWithoutAPathAlwaysAsks(t *testing.T) {
	calls := countingAPI(t, "v1.5.0", http.StatusOK)
	atTime(t, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))
	build := Build{Version: "1.4.0", Channel: "release"}

	for range 2 {
		if _, _, err := CheckCached(context.Background(), build, ""); err != nil {
			t.Fatalf("check: %v", err)
		}
	}
	if *calls != 2 {
		t.Errorf("calls = %d, want every check to be live without a memo", *calls)
	}
}

func TestLatestAuthenticatesWhenATokenIsSet(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"tag_name":"v1.5.0","html_url":"https://example.test/release"}`))
	}))
	t.Cleanup(srv.Close)
	prev := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = prev })

	t.Setenv("GITHUB_TOKEN", "")
	if _, err := Latest(context.Background()); err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want none without a token", auth)
	}

	t.Setenv("GITHUB_TOKEN", "s3cret")
	if _, err := Latest(context.Background()); err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Errorf("Authorization = %q, want a bearer token", auth)
	}
}
