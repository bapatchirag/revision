package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Project links surfaced on the Status panel's Main "about" screen.
const (
	projectDocsURL  = "https://bapatchirag.github.io/revision/"
	repoIssuesURL   = "https://github.com/bapatchirag/revision/issues"
	repoReleasesURL = "https://github.com/bapatchirag/revision/releases"
)

// revisionLogo is a compact ASCII wordmark (figlet "small") for the about screen.
const revisionLogo = `             _    _
 _ _ _____ _(_)__(_)___ _ _
| '_/ -_) V / (_-< / _ \ ' \
|_| \___|\_/|_/__/_\___/_||_|`

// versionLabel is the running binary's version as shown on the about screen: the
// release tag for an official build, and whatever the build was stamped with
// otherwise. A binary built without those stamps reports itself as development.
func (m *Model) versionLabel() string {
	v := strings.TrimSpace(m.build.Version)
	if v == "" {
		return "dev"
	}
	if m.build.IsRelease() {
		return "v" + strings.TrimPrefix(v, "v")
	}
	return v
}

// statusDetail renders the Main panel shown while the Status panel is focused:
// the logo, running version, license, project links, and a pointer to Settings.
func (m *Model) statusDetail() string {
	accent := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	label := lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true)
	link := lipgloss.NewStyle().Foreground(m.theme.Info)
	muted := lipgloss.NewStyle().Foreground(m.theme.Muted)

	note := muted.Render("Press ") + accent.Render("S") +
		muted.Render(" to review the active settings and any overrides.")

	lines := []string{
		accent.Render(revisionLogo),
		"",
		label.Render("Version"),
		"  " + accent.Render(m.versionLabel()),
		"",
		muted.Render("MIT © Chirag Bapat"),
		"",
		label.Render("Documentation"),
		"  " + link.Render(projectDocsURL),
		"",
		label.Render("Report an issue"),
		"  " + link.Render(repoIssuesURL),
		"",
		label.Render("Release notes"),
		"  " + link.Render(repoReleasesURL),
		"",
		note,
	}
	return strings.Join(lines, "\n")
}
