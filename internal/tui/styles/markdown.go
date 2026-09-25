package styles

import (
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/lipgloss"
	"github.com/svpc-ai/svpc/internal/tui/theme"
)

const defaultMargin = 1

// indentToken is the whitespace glamour inserts in front of nested blocks.
// It must not carry a background of its own, otherwise it would punch a hole
// in whichever panel the markdown is rendered on.
func indentToken() string {
	return " "
}

// Helper functions for style pointers
func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }
func uintPtr(u uint) *uint       { return &u }

// returns a glamour TermRenderer configured with the current theme
func GetMarkdownRenderer(width int) *glamour.TermRenderer {
	r, _ := glamour.NewTermRenderer(
		glamour.WithStyles(generateMarkdownStyleConfig()),
		glamour.WithWordWrap(width),
	)
	return r
}

// creates an ansi.StyleConfig for markdown rendering
// using adaptive colors from the provided theme.
func generateMarkdownStyleConfig() ansi.StyleConfig {
	t := theme.CurrentTheme()

	// Code blocks sit on their own inset surface so they read as a distinct
	// block inside the conversation. Every span (including syntax tokens)
	// carries the background explicitly, which is what keeps the block solid
	// when the surrounding message repaints its background.
	codeBackground := adaptiveColorToString(t.BackgroundDarker())
	codeSpan := func(color lipgloss.AdaptiveColor) ansi.StylePrimitive {
		return ansi.StylePrimitive{
			Color:           stringPtr(adaptiveColorToString(color)),
			BackgroundColor: stringPtr(codeBackground),
		}
	}

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "",
				Color:       stringPtr(adaptiveColorToString(t.MarkdownText())),
			},
			Margin: uintPtr(defaultMargin),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  stringPtr(adaptiveColorToString(t.MarkdownBlockQuote())),
				Italic: boolPtr(true),
				Prefix: "┃ ",
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr(indentToken()),
		},
		List: ansi.StyleList{
			LevelIndent: defaultMargin,
			StyleBlock: ansi.StyleBlock{
				IndentToken: stringPtr(indentToken()),
				StylePrimitive: ansi.StylePrimitive{
					Color: stringPtr(adaptiveColorToString(t.MarkdownText())),
				},
			},
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockSuffix: "\n",
				Color:       stringPtr(adaptiveColorToString(t.MarkdownHeading())),
				Bold:        boolPtr(true),
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "# ",
				Color:  stringPtr(adaptiveColorToString(t.MarkdownHeading())),
				Bold:   boolPtr(true),
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
				Color:  stringPtr(adaptiveColorToString(t.MarkdownHeading())),
				Bold:   boolPtr(true),
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
				Color:  stringPtr(adaptiveColorToString(t.MarkdownHeading())),
				Bold:   boolPtr(true),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
				Color:  stringPtr(adaptiveColorToString(t.MarkdownHeading())),
				Bold:   boolPtr(true),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
				Color:  stringPtr(adaptiveColorToString(t.MarkdownHeading())),
				Bold:   boolPtr(true),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  stringPtr(adaptiveColorToString(t.MarkdownHeading())),
				Bold:   boolPtr(true),
			},
		},
		Strikethrough: ansi.StylePrimitive{
			CrossedOut: boolPtr(true),
			Color:      stringPtr(adaptiveColorToString(t.TextMuted())),
		},
		Emph: ansi.StylePrimitive{
			Color:  stringPtr(adaptiveColorToString(t.MarkdownEmph())),
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold:  boolPtr(true),
			Color: stringPtr(adaptiveColorToString(t.MarkdownStrong())),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  stringPtr(adaptiveColorToString(t.MarkdownHorizontalRule())),
			Format: "\n─────────────────────────────────────────\n",
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
			Color:       stringPtr(adaptiveColorToString(t.MarkdownListItem())),
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
			Color:       stringPtr(adaptiveColorToString(t.MarkdownListEnumeration())),
		},
		Task: ansi.StyleTask{
			StylePrimitive: ansi.StylePrimitive{},
			Ticked:         "[✓] ",
			Unticked:       "[ ] ",
		},
		Link: ansi.StylePrimitive{
			Color:     stringPtr(adaptiveColorToString(t.MarkdownLink())),
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: stringPtr(adaptiveColorToString(t.MarkdownLinkText())),
			Bold:  boolPtr(true),
		},
		Image: ansi.StylePrimitive{
			Color:     stringPtr(adaptiveColorToString(t.MarkdownImage())),
			Underline: boolPtr(true),
			Format:    "🖼 {{.text}}",
		},
		ImageText: ansi.StylePrimitive{
			Color:  stringPtr(adaptiveColorToString(t.MarkdownImageText())),
			Format: "{{.text}}",
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  stringPtr(adaptiveColorToString(t.MarkdownCode())),
				Prefix: "",
				Suffix: "",
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Prefix:          " ",
					Color:           stringPtr(adaptiveColorToString(t.MarkdownCodeBlock())),
					BackgroundColor: stringPtr(codeBackground),
				},
				Margin: uintPtr(defaultMargin),
			},
			Chroma: &ansi.Chroma{
				Background:          codeSpan(t.MarkdownCodeBlock()),
				Text:                codeSpan(t.MarkdownText()),
				Error:               codeSpan(t.Error()),
				Comment:             codeSpan(t.SyntaxComment()),
				CommentPreproc:      codeSpan(t.SyntaxKeyword()),
				Keyword:             codeSpan(t.SyntaxKeyword()),
				KeywordReserved:     codeSpan(t.SyntaxKeyword()),
				KeywordNamespace:    codeSpan(t.SyntaxKeyword()),
				KeywordType:         codeSpan(t.SyntaxType()),
				Operator:            codeSpan(t.SyntaxOperator()),
				Punctuation:         codeSpan(t.SyntaxPunctuation()),
				Name:                codeSpan(t.SyntaxVariable()),
				NameBuiltin:         codeSpan(t.SyntaxVariable()),
				NameTag:             codeSpan(t.SyntaxKeyword()),
				NameAttribute:       codeSpan(t.SyntaxFunction()),
				NameClass:           codeSpan(t.SyntaxType()),
				NameConstant:        codeSpan(t.SyntaxVariable()),
				NameDecorator:       codeSpan(t.SyntaxFunction()),
				NameFunction:        codeSpan(t.SyntaxFunction()),
				LiteralNumber:       codeSpan(t.SyntaxNumber()),
				LiteralString:       codeSpan(t.SyntaxString()),
				LiteralStringEscape: codeSpan(t.SyntaxKeyword()),
				GenericDeleted:      codeSpan(t.DiffRemoved()),
				GenericInserted:     codeSpan(t.DiffAdded()),
				GenericSubheading:   codeSpan(t.MarkdownHeading()),
				GenericEmph: ansi.StylePrimitive{
					Color:           stringPtr(adaptiveColorToString(t.MarkdownEmph())),
					BackgroundColor: stringPtr(codeBackground),
					Italic:          boolPtr(true),
				},
				GenericStrong: ansi.StylePrimitive{
					Color:           stringPtr(adaptiveColorToString(t.MarkdownStrong())),
					BackgroundColor: stringPtr(codeBackground),
					Bold:            boolPtr(true),
				},
			},
		},
		Table: ansi.StyleTable{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
				},
			},
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
		DefinitionDescription: ansi.StylePrimitive{
			BlockPrefix: "\n ❯ ",
			Color:       stringPtr(adaptiveColorToString(t.MarkdownLinkText())),
		},
		Text: ansi.StylePrimitive{
			Color: stringPtr(adaptiveColorToString(t.MarkdownText())),
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr(adaptiveColorToString(t.MarkdownText())),
			},
		},
	}
}

// adaptiveColorToString converts a lipgloss.AdaptiveColor to the appropriate
// hex color string based on the current terminal background
func adaptiveColorToString(color lipgloss.AdaptiveColor) string {
	if lipgloss.HasDarkBackground() {
		return color.Dark
	}
	return color.Light
}
