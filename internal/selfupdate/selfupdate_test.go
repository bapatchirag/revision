package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"testing"
)

func TestIsRelease(t *testing.T) {
	cases := []struct {
		name  string
		build Build
		want  bool
	}{
		{"release semver", Build{Version: "1.4.0", Channel: "release"}, true},
		{"release v-prefixed", Build{Version: "v1.4.0", Channel: "release"}, true},
		{"release prerelease", Build{Version: "1.4.0-rc.1", Channel: "release"}, true},
		{"dev channel", Build{Version: "1.4.0", Channel: "dev"}, false},
		{"empty channel", Build{Version: "1.4.0", Channel: ""}, false},
		// A locally cross-compiled build carries a git-describe version but the
		// release pipeline never marks it, so the channel keeps it out.
		{"describe version, dev channel", Build{Version: "v1.4.0-3-gabc123", Channel: "dev"}, false},
		{"release but dev version", Build{Version: "dev", Channel: "release"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.build.IsRelease(); got != tc.want {
				t.Errorf("IsRelease(%+v) = %v, want %v", tc.build, got, tc.want)
			}
		})
	}
}

func TestIsPseudoVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    bool
	}{
		{"no base version", "v0.0.0-20260101120000-abcdef123456", true},
		{"release base", "v1.4.1-0.20260101120000-abcdef123456", true},
		{"prerelease base", "v1.4.0-rc.1.0.20260101120000-abcdef123456", true},
		{"release tag", "v1.4.0", false},
		{"prerelease tag", "1.4.0-rc.1", false},
		{"devel sentinel", "(devel)", false},
		{"short revision", "v0.0.0-20260101120000-abcdef", false},
		{"short timestamp", "v0.0.0-2026010112-abcdef123456", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPseudoVersion(tc.version); got != tc.want {
				t.Errorf("isPseudoVersion(%q) = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}

func TestResolveFromModule(t *testing.T) {
	dev := Build{Version: "dev", Channel: "dev"}
	fromVCS := []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.modified", Value: "false"}}
	cases := []struct {
		name     string
		path     string
		version  string
		settings []debug.BuildSetting
		want     Build
	}{
		{"published tag", module, "v1.4.0", nil, Build{Version: "1.4.0", Channel: "release"}},
		{"published prerelease", module, "v1.4.0-rc.1", nil, Build{Version: "1.4.0-rc.1", Channel: "release"}},
		{"devel sentinel", module, "(devel)", nil, dev},
		{"untagged commit", module, "v0.0.0-20260101120000-abcdef123456", nil, dev},
		// Since Go 1.24 a working-tree build reports the checkout's nearest tag,
		// so only the vcs settings keep `make build` out of the release channel.
		{"local build at a tag", module, "v1.4.0", fromVCS, dev},
		{"local build, dirty", module, "v1.4.0+dirty", fromVCS, dev},
		// A fork publishes its own tags; they say nothing about this project's
		// releases, so its binaries must stay inert.
		{"fork at a tag", "github.com/someone/revision", "v9.9.9", nil, dev},
		{"no module recorded", "", "", nil, dev},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := &debug.BuildInfo{
				Main:     debug.Module{Path: tc.path, Version: tc.version},
				Settings: tc.settings,
			}
			if got := resolveFromModule(dev, info); got != tc.want {
				t.Errorf("resolveFromModule(%q, %q) = %+v, want %+v", tc.path, tc.version, got, tc.want)
			}
		})
	}
}

func TestResolveKeepsAStampedBuild(t *testing.T) {
	// The release pipeline is authoritative: its values are never second-guessed
	// against the build info.
	stamped := Build{Version: "1.4.0", Channel: "release"}
	if got := Resolve(stamped.Version, stamped.Channel); got != stamped {
		t.Errorf("Resolve(%+v) = %+v, want it unchanged", stamped, got)
	}
	// The test binary is built from this working tree, so an unstamped build
	// stays unstamped.
	dev := Build{Version: "dev", Channel: "dev"}
	if got := Resolve(dev.Version, dev.Channel); got != dev {
		t.Errorf("Resolve(%+v) = %+v, want it unchanged", dev, got)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.4.0", "1.3.9", 1},
		{"1.3.9", "1.4.0", -1},
		{"1.4.0", "1.4.0", 0},
		{"v1.4.0", "1.4.0", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.4.1", "1.4.0", 1},
		{"1.4.0", "1.4.0-rc.1", 1},  // release beats prerelease
		{"1.4.0-rc.1", "1.4.0", -1}, // prerelease loses to release
		{"1.4.0-rc.2", "1.4.0-rc.1", 1},
		{"1.4.0+build.9", "1.4.0", 0}, // build metadata ignored
	}
	for _, tc := range cases {
		got, err := compareVersions(tc.a, tc.b)
		if err != nil {
			t.Fatalf("compareVersions(%q,%q): unexpected error %v", tc.a, tc.b, err)
		}
		if got != tc.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCompareVersionsInvalid(t *testing.T) {
	if _, err := compareVersions("dev", "1.0.0"); err == nil {
		t.Error("expected an error comparing a non-semver version")
	}
}

func TestParseSemverRejectsSentinel(t *testing.T) {
	if _, ok := parseSemver("dev"); ok {
		t.Error("dev must not parse as a semver")
	}
	if _, ok := parseSemver("1.4"); ok {
		t.Error("a two-part version must not parse as a semver")
	}
	if _, ok := parseSemver("v1.4.0"); !ok {
		t.Error("a v-prefixed three-part version should parse")
	}
}

// withAPI points the package at a stub GitHub server for the duration of a test.
func withAPI(t *testing.T, tagName string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+repo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"tag_name":"` + tagName + `","html_url":"https://example.test/release"}`))
		}
	}))
	t.Cleanup(srv.Close)

	prev := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = prev })
}

func TestCheckReportsNewerRelease(t *testing.T) {
	withAPI(t, "v1.5.0", http.StatusOK)

	rel, newer, err := Check(context.Background(), Build{Version: "1.4.0", Channel: "release"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !newer {
		t.Error("expected a newer release to be reported")
	}
	if rel.Tag != "v1.5.0" || rel.Version != "1.5.0" {
		t.Errorf("got tag=%q version=%q, want v1.5.0/1.5.0", rel.Tag, rel.Version)
	}
}

func TestCheckReportsUpToDate(t *testing.T) {
	withAPI(t, "v1.4.0", http.StatusOK)

	_, newer, err := Check(context.Background(), Build{Version: "1.4.0", Channel: "release"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if newer {
		t.Error("expected no update when versions match")
	}
}

func TestCheckSkipsDevBuildsWithoutNetwork(t *testing.T) {
	// Point the API at a dead address; a dev build must not touch it.
	prev := apiBase
	apiBase = "http://127.0.0.1:0"
	t.Cleanup(func() { apiBase = prev })

	_, newer, err := Check(context.Background(), Build{Version: "dev", Channel: "dev"})
	if err != nil {
		t.Fatalf("dev build Check should not error, got %v", err)
	}
	if newer {
		t.Error("dev build must never report an available update")
	}
}

func TestCheckSurfacesHTTPError(t *testing.T) {
	withAPI(t, "", http.StatusInternalServerError)

	if _, _, err := Check(context.Background(), Build{Version: "1.4.0", Channel: "release"}); err == nil {
		t.Error("expected an error on a non-200 response")
	}
}

func TestLatestRejectsAnUnusableTag(t *testing.T) {
	// The tag becomes a path segment of the install-script URL and an argument
	// to `go install`, so it is checked before it is used for either.
	for _, tag := range []string{"../../elsewhere/main", "v1.4.0 rm -rf /", "-v1.4.0"} {
		t.Run(tag, func(t *testing.T) {
			withAPI(t, tag, http.StatusOK)
			if _, err := Latest(context.Background()); err == nil {
				t.Errorf("Latest() accepted the tag %q", tag)
			}
		})
	}
}

func TestMethodLabel(t *testing.T) {
	if MethodCurl.Label() != "curl" {
		t.Errorf("MethodCurl.Label() = %q", MethodCurl.Label())
	}
	if MethodGo.Label() != "go install" {
		t.Errorf("MethodGo.Label() = %q", MethodGo.Label())
	}
}
