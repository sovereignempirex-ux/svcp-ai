package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/svpc-ai/svpc/internal/tui/theme"
)

// Design tokens for the OpenCode interface.
//
// This file is the single source of truth for the visual language of the app:
// spacing, borders, surfaces, shared chrome (panels, dialogs, chips, empty
// states) and the small typographic details that make the UI feel consistent.
//
// Components must compose these helpers instead of hand-rolling their own
// lipgloss styles, so that every screen inherits the same look and a single
// change here restyles the whole product.

// ---------------------------------------------------------------------------
// Spacing scale (terminal cells)
// ---------------------------------------------------------------------------

const (
	SpaceNone = 0
	Space1    = 1 // inline gap
	Space2    = 2 // gap between related elements
	Space3    = 3 // gap between blocks
	Space4    = 4 // section separation
	Space6    = 6 // dialog / panel gutters
)

// ---------------------------------------------------------------------------
// Borders
// ---------------------------------------------------------------------------

var (
	// BorderSoft is the default chrome for framed surfaces (dialogs, cards).
	BorderSoft = lipgloss.RoundedBorder()

	// BorderLine is the plain hairline used for single edge separators.
	BorderLine = lipgloss.NormalBorder()

	// BorderAccent is used to emphasise a focused edge or a highlighted block.
	BorderAccent = lipgloss.ThickBorder()
)

// ---------------------------------------------------------------------------
// Surfaces
// ---------------------------------------------------------------------------

// Surface is the elevated "glass" background used for panels that sit on top
// of the application background (sidebar, status bar, dialogs). It is exactly
// one step above Background() so depth reads without adding noise.
func Surface() lipgloss.Style {
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Background(t.BackgroundSecondary()).
		Foreground(t.Text())
}

// Transparent is a text-only base: it sets the foreground but leaves the
// background alone so the element inherits whatever surface its parent painted
// (canvas, glass panel, dialog...). Content that can be rendered on more than
// one surface must use this instead of BaseStyle, otherwise it stamps the
// canvas colour over the panel it sits on.
func Transparent() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme().Text())
}

// Panel is a framed glass panel: soft corners, hairline border, base
// background. Use it for anything that should read as a "card".
func Panel() lipgloss.Style {
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Border(BorderSoft).
		BorderForeground(t.BorderNormal()).
		BorderBackground(t.Background()).
		Background(t.Background()).
		Foreground(t.Text())
}

// PanelFocused is Panel() with the accent border, used for the pane that
// currently owns keyboard focus.
func PanelFocused() lipgloss.Style {
	t := theme.CurrentTheme()
	return Panel().
		BorderForeground(t.BorderFocused())
}

// DialogFrame is the standard dialog chrome. Pass the content width you want
// inside the padding; the padding and border are applied on top of it exactly
// like the classic dialog frame, so existing layouts keep their size.
func DialogFrame(width int) lipgloss.Style {
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Border(BorderSoft).
		BorderForeground(t.BorderNormal()).
		BorderBackground(t.Background()).
		Background(t.Background()).
		Foreground(t.Text()).
		Padding(Space1, Space2).
		Width(width)
}

// ---------------------------------------------------------------------------
// Typography helpers
// ---------------------------------------------------------------------------

// SectionLabel renders a small section heading used by sidebars and panels:
// an accent marker followed by an uppercase, muted label. Terminals have no
// letter spacing, so hierarchy comes from case, weight and the marker.
func SectionLabel(text string) string {
	t := theme.CurrentTheme()
	marker := lipgloss.NewStyle().
		Foreground(t.Primary()).
		Render(SectionIcon)
	label := lipgloss.NewStyle().
		Foreground(t.TextMuted()).
		Bold(true).
		Render(strings.ToUpper(text))
	return marker + " " + label
}

// SectionLabelMuted is SectionLabel() without the accent marker, for groups
// that are secondary to the surrounding content.
func SectionLabelMuted(text string) string {
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Foreground(t.TextMuted()).
		Bold(true).
		Render(strings.ToUpper(text))
}

// Divider renders a hairline rule across the given width.
func Divider(width int) string {
	if width < 1 {
		return ""
	}
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Foreground(t.BorderDim()).
		Render(strings.Repeat("─", width))
}

// DividerDashed is the lighter, dotted-in-feel rule for dense areas.
func DividerDashed(width int) string {
	if width < 1 {
		return ""
	}
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Foreground(t.BorderDim()).
		Render(strings.Repeat("╌", width))
}

// ---------------------------------------------------------------------------
// Interactive chrome
// ---------------------------------------------------------------------------

// KeyCap renders a keyboard shortcut as a physical-looking key.
func KeyCap(label string) string {
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Background(t.BackgroundSecondary()).
		Foreground(t.TextEmphasized()).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderDim()).
		Padding(0, 1).
		Render(label)
}

// Chip renders a compact status chip with an explicit background/foreground
// pair. Used by the status bar and tool/diff badges.
func Chip(text string, bg, fg lipgloss.AdaptiveColor) string {
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(fg).
		Bold(true).
		Padding(0, 1).
		Render(text)
}

// ChipMuted renders a low emphasis chip on the elevated surface.
func ChipMuted(text string) string {
	t := theme.CurrentTheme()
	return lipgloss.NewStyle().
		Background(t.BackgroundSecondary()).
		Foreground(t.TextMuted()).
		Padding(0, 1).
		Render(text)
}

// Badge renders a tiny inline counter/diff badge (files changed, +n/-n...).
func Badge(text string, color lipgloss.AdaptiveColor) string {
	return lipgloss.NewStyle().
		Foreground(color).
		Bold(true).
		Render(text)
}

// ---------------------------------------------------------------------------
// Shared blocks
// ---------------------------------------------------------------------------

// EmptyState renders a centred, low contrast placeholder for panels that have
// nothing to show yet. width is the usable width of the panel.
func EmptyState(width int, icon, title, hint string) string {
	t := theme.CurrentTheme()
	if width < 1 {
		width = 1
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().
		Foreground(t.BorderFocused()).
		Width(width).
		Align(lipgloss.Center).
		Render(icon))

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(t.TextMuted()).
		Width(width).
		Align(lipgloss.Center).
		Render(title))

	if hint != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(t.BorderDim()).
			Width(width).
			Align(lipgloss.Center).
			Render(hint))
	}

	return b.String()
}
