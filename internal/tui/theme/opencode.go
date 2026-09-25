package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// OpenCodeTheme implements the Theme interface with OpenCode brand colors.
// It provides both dark and light variants.
//
// The palette is built around a deep, blue-tinted near-black canvas with a
// single bright accent (cyan) plus a restrained secondary (violet) and warm
// highlight. Status colours only surface when they carry meaning, so the
// screen stays calm during long sessions.
type OpenCodeTheme struct {
	BaseTheme
}

// NewOpenCodeTheme creates a new instance of the OpenCode theme.
func NewOpenCodeTheme() *OpenCodeTheme {
	// Dark mode colors
	darkBackground := "#0A0C11"  // deep canvas
	darkCurrentLine := "#10131A" // elevated surface (glass panels)
	darkSelection := "#181D26"   // hover / dim chrome
	darkForeground := "#E7EAF2"  // primary text
	darkComment := "#7C8497"     // muted text
	darkPrimary := "#4CC2FF"     // primary accent (cyan)
	darkSecondary := "#9B8CFF"   // secondary accent (violet)
	darkAccent := "#2DD4BF"      // tertiary accent (teal)
	darkRed := "#F87171"         // Error red
	darkOrange := "#FBBF24"      // Warning amber
	darkGreen := "#34D399"       // Success green
	darkCyan := "#38BDF8"        // Info cyan
	darkYellow := "#E6C885"      // Emphasized text (warm sand)
	darkBorder := "#2A3140"      // hairline border

	// Light mode colors
	lightBackground := "#F7F8FA"
	lightCurrentLine := "#EDEFF4"
	lightSelection := "#E2E6EE"
	lightForeground := "#1B1F2A"
	lightComment := "#6B7280"
	lightPrimary := "#0B76C4"   // Primary blue
	lightSecondary := "#6D4AE0" // Secondary purple
	lightAccent := "#0F9B8E"    // Accent teal
	lightRed := "#D1383D"       // Error red
	lightOrange := "#C77700"    // Warning orange
	lightGreen := "#2E8B57"     // Success green
	lightCyan := "#1F7FA8"      // Info cyan
	lightYellow := "#9A7412"    // Emphasized text
	lightBorder := "#D4D8E0"    // Border color

	theme := &OpenCodeTheme{}

	// Base colors
	theme.PrimaryColor = lipgloss.AdaptiveColor{
		Dark:  darkPrimary,
		Light: lightPrimary,
	}
	theme.SecondaryColor = lipgloss.AdaptiveColor{
		Dark:  darkSecondary,
		Light: lightSecondary,
	}
	theme.AccentColor = lipgloss.AdaptiveColor{
		Dark:  darkAccent,
		Light: lightAccent,
	}

	// Status colors
	theme.ErrorColor = lipgloss.AdaptiveColor{
		Dark:  darkRed,
		Light: lightRed,
	}
	theme.WarningColor = lipgloss.AdaptiveColor{
		Dark:  darkOrange,
		Light: lightOrange,
	}
	theme.SuccessColor = lipgloss.AdaptiveColor{
		Dark:  darkGreen,
		Light: lightGreen,
	}
	theme.InfoColor = lipgloss.AdaptiveColor{
		Dark:  darkCyan,
		Light: lightCyan,
	}

	// Text colors
	theme.TextColor = lipgloss.AdaptiveColor{
		Dark:  darkForeground,
		Light: lightForeground,
	}
	theme.TextMutedColor = lipgloss.AdaptiveColor{
		Dark:  darkComment,
		Light: lightComment,
	}
	theme.TextEmphasizedColor = lipgloss.AdaptiveColor{
		Dark:  darkYellow,
		Light: lightYellow,
	}

	// Background colors
	theme.BackgroundColor = lipgloss.AdaptiveColor{
		Dark:  darkBackground,
		Light: lightBackground,
	}
	theme.BackgroundSecondaryColor = lipgloss.AdaptiveColor{
		Dark:  darkCurrentLine,
		Light: lightCurrentLine,
	}
	theme.BackgroundDarkerColor = lipgloss.AdaptiveColor{
		Dark:  "#06080C", // deepest layer (overlay shadows)
		Light: "#FFFFFF",
	}

	// Border colors
	theme.BorderNormalColor = lipgloss.AdaptiveColor{
		Dark:  darkBorder,
		Light: lightBorder,
	}
	theme.BorderFocusedColor = lipgloss.AdaptiveColor{
		Dark:  darkPrimary,
		Light: lightPrimary,
	}
	theme.BorderDimColor = lipgloss.AdaptiveColor{
		Dark:  darkSelection,
		Light: lightSelection,
	}

	// Diff view colors
	theme.DiffAddedColor = lipgloss.AdaptiveColor{
		Dark:  "#7EE787",
		Light: "#1B7F37",
	}
	theme.DiffRemovedColor = lipgloss.AdaptiveColor{
		Dark:  "#FF7B72",
		Light: "#B3261E",
	}
	theme.DiffContextColor = lipgloss.AdaptiveColor{
		Dark:  "#8B94A6",
		Light: "#5B6270",
	}
	theme.DiffHunkHeaderColor = lipgloss.AdaptiveColor{
		Dark:  "#9B8CFF",
		Light: "#6D4AE0",
	}
	theme.DiffHighlightAddedColor = lipgloss.AdaptiveColor{
		Dark:  "#D6F7DF",
		Light: "#A9DFBC",
	}
	theme.DiffHighlightRemovedColor = lipgloss.AdaptiveColor{
		Dark:  "#FDDCDA",
		Light: "#F3B7B3",
	}
	theme.DiffAddedBgColor = lipgloss.AdaptiveColor{
		Dark:  "#0F2116",
		Light: "#E7F6EC",
	}
	theme.DiffRemovedBgColor = lipgloss.AdaptiveColor{
		Dark:  "#2A1315",
		Light: "#FCEBEA",
	}
	theme.DiffContextBgColor = lipgloss.AdaptiveColor{
		Dark:  darkBackground,
		Light: lightBackground,
	}
	theme.DiffLineNumberColor = lipgloss.AdaptiveColor{
		Dark:  "#5C6478",
		Light: "#9AA1B0",
	}
	theme.DiffAddedLineNumberBgColor = lipgloss.AdaptiveColor{
		Dark:  "#0C1A11",
		Light: "#D6EEDF",
	}
	theme.DiffRemovedLineNumberBgColor = lipgloss.AdaptiveColor{
		Dark:  "#201013",
		Light: "#F8D7D5",
	}

	// Markdown colors
	theme.MarkdownTextColor = lipgloss.AdaptiveColor{
		Dark:  darkForeground,
		Light: lightForeground,
	}
	theme.MarkdownHeadingColor = lipgloss.AdaptiveColor{
		Dark:  darkPrimary,
		Light: lightPrimary,
	}
	theme.MarkdownLinkColor = lipgloss.AdaptiveColor{
		Dark:  darkSecondary,
		Light: lightSecondary,
	}
	theme.MarkdownLinkTextColor = lipgloss.AdaptiveColor{
		Dark:  darkCyan,
		Light: lightCyan,
	}
	theme.MarkdownCodeColor = lipgloss.AdaptiveColor{
		Dark:  "#7EE787",
		Light: "#1B7F37",
	}
	theme.MarkdownBlockQuoteColor = lipgloss.AdaptiveColor{
		Dark:  darkComment,
		Light: lightComment,
	}
	theme.MarkdownEmphColor = lipgloss.AdaptiveColor{
		Dark:  darkYellow,
		Light: lightYellow,
	}
	theme.MarkdownStrongColor = lipgloss.AdaptiveColor{
		Dark:  darkAccent,
		Light: lightAccent,
	}
	theme.MarkdownHorizontalRuleColor = lipgloss.AdaptiveColor{
		Dark:  darkBorder,
		Light: lightBorder,
	}
	theme.MarkdownListItemColor = lipgloss.AdaptiveColor{
		Dark:  darkPrimary,
		Light: lightPrimary,
	}
	theme.MarkdownListEnumerationColor = lipgloss.AdaptiveColor{
		Dark:  darkSecondary,
		Light: lightSecondary,
	}
	theme.MarkdownImageColor = lipgloss.AdaptiveColor{
		Dark:  darkPrimary,
		Light: lightPrimary,
	}
	theme.MarkdownImageTextColor = lipgloss.AdaptiveColor{
		Dark:  darkCyan,
		Light: lightCyan,
	}
	theme.MarkdownCodeBlockColor = lipgloss.AdaptiveColor{
		Dark:  darkForeground,
		Light: lightForeground,
	}

	// Syntax highlighting colors
	theme.SyntaxCommentColor = lipgloss.AdaptiveColor{
		Dark:  darkComment,
		Light: lightComment,
	}
	theme.SyntaxKeywordColor = lipgloss.AdaptiveColor{
		Dark:  "#FF7EB6",
		Light: "#A3175E",
	}
	theme.SyntaxFunctionColor = lipgloss.AdaptiveColor{
		Dark:  darkPrimary,
		Light: lightPrimary,
	}
	theme.SyntaxVariableColor = lipgloss.AdaptiveColor{
		Dark:  darkForeground,
		Light: lightForeground,
	}
	theme.SyntaxStringColor = lipgloss.AdaptiveColor{
		Dark:  "#7EE787",
		Light: "#1B7F37",
	}
	theme.SyntaxNumberColor = lipgloss.AdaptiveColor{
		Dark:  "#FFB86C",
		Light: "#B45309",
	}
	theme.SyntaxTypeColor = lipgloss.AdaptiveColor{
		Dark:  darkYellow,
		Light: lightYellow,
	}
	theme.SyntaxOperatorColor = lipgloss.AdaptiveColor{
		Dark:  "#89DDFF",
		Light: "#0B76C4",
	}
	theme.SyntaxPunctuationColor = lipgloss.AdaptiveColor{
		Dark:  "#9AA4B2",
		Light: "#5B6270",
	}

	return theme
}

func init() {
	// Register the OpenCode theme with the theme manager
	RegisterTheme("svpc", NewOpenCodeTheme())
}
