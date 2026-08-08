// Package selfupdate checks GitHub for a newer released binary and performs the
// upgrade through one of the project's documented install paths (the install.sh
// script via curl, or `go install`). It is deliberately domain-agnostic: it
// knows nothing about the TUI or the SVN layer, so both the running app and the
// `--update` CLI path can share it.
//
// Update checks are gated on the build channel: only official release builds
// (marked by the release pipeline) ever contact GitHub or upgrade themselves.
// Development and locally cross-compiled builds are inert.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const (
	// repo is the GitHub "owner/name" this binary updates from.
	repo = "bapatchirag/revision"

	// module is this project's Go module path, as recorded in the build info of
	// any binary produced from it.
	module = "github.com/" + repo

	// installURL is the raw install script used by the curl update path. It is
	// the same one-liner documented in the README and installs the latest
	// release for the host OS/arch.
	installURL = "https://raw.githubusercontent.com/bapatchirag/revision/main/install.sh"

	// goModule is the module path used by the `go install` update path.
	goModule = module + "/cmd/revision"

	// releaseChannel is the channel value that marks an official release build.
	// Any other value (e.g. "dev") is treated as a development build and never
	// checks for or applies updates.
	releaseChannel = "release"
)

// apiBase is the GitHub REST API root. It is a variable so tests can point it at
// a local server.
var apiBase = "https://api.github.com"

// httpClient bounds every update check so a slow or unreachable network can
// never hang startup; callers still pass a context for finer control.
var httpClient = &http.Client{Timeout: 8 * time.Second}

// executablePath locates the running binary. It is a variable so tests can
// stand a temporary path in for it.
var executablePath = os.Executable

// installDir is the directory an update has to land in: the one the running
// binary occupies. Symlinks are resolved so the update replaces the real file
// rather than leaving a stale copy behind the link.
func installDir() (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("locating the running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

// Method identifies how an update is applied.
type Method int

const (
	// MethodCurl pipes the install script through the shell (curl | sh).
	MethodCurl Method = iota
	// MethodGo runs `go install <module>@latest`.
	MethodGo
)

// Build describes the running binary's provenance, injected at build time.
type Build struct {
	Version string // semver such as "1.4.0" on releases; "dev" or a git-describe string otherwise
	Channel string // "release" for official builds; anything else is a development build
}

// IsRelease reports whether this is an official release build eligible for
// update checks. A build qualifies only when the release pipeline marked its
// channel and stamped a parseable semver version, so development and
// locally cross-compiled builds are always excluded.
func (b Build) IsRelease() bool {
	if b.Channel != releaseChannel {
		return false
	}
	_, ok := parseSemver(b.Version)
	return ok
}

// Resolve reports the running binary's provenance from the values the release
// pipeline stamps into it, falling back to the module version the Go toolchain
// records when there are none. Without that fallback a binary produced by
// `go install <module>@<tag>` — a documented install path — would carry the
// development defaults and be permanently barred from updating itself.
func Resolve(version, channel string) Build {
	b := Build{Version: version, Channel: channel}
	if b.IsRelease() {
		return b
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	return resolveFromModule(b, info)
}

// resolveFromModule promotes a build to the release channel when the toolchain
// recorded this module at a published tag. A fork, the "(devel)" sentinel, and
// the pseudo-version of an untagged commit all leave the build exactly as the
// linker left it. So does anything built from a working tree: since Go 1.24 the
// toolchain derives Main.Version from the checkout's own tags, so only the vcs
// settings — which a module-cache build never carries — tell the two apart.
func resolveFromModule(b Build, info *debug.BuildInfo) Build {
	if info.Main.Path != module || isPseudoVersion(info.Main.Version) {
		return b
	}
	if _, ok := parseSemver(info.Main.Version); !ok {
		return b
	}
	for _, s := range info.Settings {
		if strings.HasPrefix(s.Key, "vcs") {
			return b
		}
	}
	return Build{Version: strings.TrimPrefix(info.Main.Version, "v"), Channel: releaseChannel}
}

// Release is the metadata for the latest published GitHub release.
type Release struct {
	Tag     string // release tag, e.g. "v1.4.0"
	Version string // tag with any leading "v" stripped, e.g. "1.4.0"
	URL     string // human-facing release page
}

// Check returns the latest release and whether it is newer than the running
// build. On a development build it reports no update without any network call.
func Check(ctx context.Context, b Build) (Release, bool, error) {
	if !b.IsRelease() {
		return Release{}, false, nil
	}
	rel, err := Latest(ctx)
	if err != nil {
		return Release{}, false, err
	}
	cmp, err := compareVersions(rel.Version, b.Version)
	if err != nil {
		return rel, false, err
	}
	return rel, cmp > 0, nil
}

// Latest fetches the most recent published release from the GitHub API.
func Latest(ctx context.Context) (Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "revision-selfupdate")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github: unexpected status %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	// Bound the body so a malformed or hostile response cannot exhaust memory.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("github: decode response: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, errors.New("github: latest release has no tag")
	}
	return Release{
		Tag:     payload.TagName,
		Version: strings.TrimPrefix(payload.TagName, "v"),
		URL:     payload.HTMLURL,
	}, nil
}

// Run applies an update using the chosen method, pinned to the release the user
// approved and aimed at the directory the running binary occupies, and streams
// the underlying command's output to the current terminal. It fails fast with a
// clear message when the required tool (curl or go) is not on the PATH.
func Run(m Method, rel Release) error {
	if rel.Tag == "" {
		return errors.New("no release to install")
	}
	dir, err := installDir()
	if err != nil {
		return err
	}
	switch m {
	case MethodGo:
		// Without GOBIN the new binary lands in $GOPATH/bin, leaving the running
		// one untouched and the PATH order to decide which of the two wins.
		env := []string{"GOBIN=" + dir}
		return execUpdate("go", env, "go", "install", goModule+"@"+rel.Tag)
	default:
		// install.sh takes both from the environment, so the script installs the
		// release that was announced, over the binary that asked for it, instead
		// of re-resolving "latest" into a directory of its own choosing.
		env := []string{
			"REVISION_VERSION=" + rel.Tag,
			"REVISION_INSTALL_DIR=" + dir,
		}
		return execUpdate("curl", env, "sh", "-c", "curl -fsSL "+installURL+" | sh")
	}
}

// Label returns a short human name for a method, used in prompts and messages.
func (m Method) Label() string {
	switch m {
	case MethodGo:
		return "go install"
	default:
		return "curl"
	}
}

// execUpdate checks that require is installed, then runs name/args with the
// process's own stdio so the user sees live progress. env adds to the process
// environment the child inherits.
func execUpdate(require string, env []string, name string, args ...string) error {
	if _, err := exec.LookPath(require); err != nil {
		return fmt.Errorf("%s is required for this update method but was not found on your PATH", require)
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

// semver is a parsed MAJOR.MINOR.PATCH with an optional pre-release.
type semver struct {
	major, minor, patch int
	pre                 string
}

// parseSemver parses a version like "v1.4.0", "1.4.0" or "1.4.0-rc.1". Build
// metadata (after "+") is ignored. It reports ok=false for anything that is not
// a three-part numeric core, which includes the "dev" sentinel.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core := s
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core, pre = s[:i], s[i+1:]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := atoiStrict(p)
		if err != nil {
			return semver{}, false
		}
		nums[i] = n
	}
	return semver{major: nums[0], minor: nums[1], patch: nums[2], pre: pre}, true
}

// pseudoVersionRE matches the timestamp and commit the Go toolchain appends
// when it synthesises a version for an untagged commit, in all three of the
// forms it produces.
var pseudoVersionRE = regexp.MustCompile(`(^|\.)[0-9]{14}-[0-9a-f]{12}$`)

// isPseudoVersion reports whether v is a Go module pseudo-version, which stands
// for a commit that carries no release tag.
func isPseudoVersion(v string) bool {
	sv, ok := parseSemver(v)
	return ok && pseudoVersionRE.MatchString(sv.pre)
}

// atoiStrict parses a run of ASCII digits, rejecting signs, spaces and empties
// so a git-describe fragment never masquerades as a version component.
func atoiStrict(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit in %q", s)
		}
	}
	return strconv.Atoi(s)
}

// compareVersions returns >0 if a is newer than b, 0 if equal, <0 if older.
func compareVersions(a, b string) (int, error) {
	av, ok := parseSemver(a)
	if !ok {
		return 0, fmt.Errorf("invalid version %q", a)
	}
	bv, ok := parseSemver(b)
	if !ok {
		return 0, fmt.Errorf("invalid version %q", b)
	}
	for _, d := range [][2]int{{av.major, bv.major}, {av.minor, bv.minor}, {av.patch, bv.patch}} {
		if d[0] != d[1] {
			return sign(d[0] - d[1]), nil
		}
	}
	return comparePre(av.pre, bv.pre), nil
}

// comparePre orders pre-release identifiers: a build with no pre-release
// outranks one with a pre-release (1.0.0 > 1.0.0-rc.1); otherwise they compare
// lexically, which is sufficient for the common numeric/rc cases.
func comparePre(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	case a == b:
		return 0
	case a < b:
		return -1
	default:
		return 1
	}
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}
