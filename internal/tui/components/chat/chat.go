package chat

import (
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/message"
	"github.com/svpc-ai/svpc/internal/session"
	"github.com/svpc-ai/svpc/internal/tui/styles"
	"github.com/svpc-ai/svpc/internal/tui/theme"
	"github.com/svpc-ai/svpc/internal/version"
)

type SendMsg struct {
	Text        string
	Attachments []message.Attachment
}

type SessionSelectedMsg = session.Session

type SessionClearedMsg struct{}

type EditorFocusMsg bool

func header(width int) string {
	return lipgloss.JoinVertical(
		lipgloss.Top,
		logo(width),
		repo(width),
		"",
		cwd(width),
	)
}

func lspsConfigured(width int) string {
	cfg := config.Get()
	t := theme.CurrentTheme()
	base := styles.Transparent()

	section := styles.SectionLabel("LSP configuration")

	// Get LSP names and sort them for consistent ordering
	var lspNames []string
	for name := range cfg.LSP {
		lspNames = append(lspNames, name)
	}
	sort.Strings(lspNames)

	if len(lspNames) == 0 {
		return base.
			Width(width).
			Render(
				lipgloss.JoinVertical(
					lipgloss.Left,
					section,
					base.Foreground(t.TextMuted()).Render("No language servers configured"),
				),
			)
	}

	var lspViews []string
	for _, name := range lspNames {
		lsp := cfg.LSP[name]

		marker := base.Foreground(t.BorderDim()).Render(styles.BulletIcon)
		lspName := base.Foreground(t.Text()).Render(" " + name)

		cmdWidth := width - lipgloss.Width(marker) - lipgloss.Width(lspName) - 2
		if cmdWidth < 1 {
			cmdWidth = 1
		}
		lspPath := base.
			Foreground(t.TextMuted()).
			Render(" " + ansi.Truncate(lsp.Command, cmdWidth, "…"))

		lspViews = append(lspViews,
			base.
				Width(width).
				Render(
					lipgloss.JoinHorizontal(
						lipgloss.Left,
						marker,
						lspName,
						lspPath,
					),
				),
		)
	}

	return base.
		Width(width).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				append([]string{section}, lspViews...)...,
			),
		)
}

func logo(width int) string {
	t := theme.CurrentTheme()
	base := styles.Transparent()

	brand := base.
		Foreground(t.Primary()).
		Render(styles.OpenCodeIcon)
	name := base.
		Bold(true).
		Foreground(t.Text()).
		Render(" OpenCode")

	versionText := base.
		Foreground(t.TextMuted()).
		Render(" " + version.Version)

	return base.
		Width(width).
		Render(
			lipgloss.JoinHorizontal(
				lipgloss.Left,
				brand,
				name,
				versionText,
			),
		)
}

func repo(width int) string {
	repo := "https://github.com/svpc-ai/svpc"
	t := theme.CurrentTheme()

	return styles.Transparent().
		Foreground(t.TextMuted()).
		Width(width).
		Render(ansi.Truncate(repo, width, "…"))
}

func cwd(width int) string {
	t := theme.CurrentTheme()
	base := styles.Transparent()

	label := base.
		Foreground(t.TextMuted()).
		Bold(true).
		Render("cwd")

	pathWidth := width - lipgloss.Width(label) - 1
	if pathWidth < 1 {
		pathWidth = 1
	}
	path := base.
		Foreground(t.Text()).
		Render(ansi.Truncate(config.WorkingDirectory(), pathWidth, "…"))

	return base.
		Width(width).
		Render(lipgloss.JoinHorizontal(lipgloss.Left, label, " ", path))
}
