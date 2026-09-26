package styles

// Icon set for the interface.
//
// All glyphs are single code points chosen to stay monochrome (so they inherit
// the theme colour instead of rendering as coloured emoji) and to occupy one
// terminal cell whenever possible, which keeps width calculations predictable.
const (
	// Brand
	BrandIcon   string = "⌬"
	SparkIcon   string = "✦" // AI / agent accents
	SectionIcon string = "◆" // sidebar + panel section markers
	DotIcon     string = "●" // status dot (idle / ok / busy)
	BulletIcon  string = "▸" // list bullet
	ChevronIcon string = "›" // disclosure / navigation
	ArrowIcon   string = "→" // "go to" hint
	PromptIcon  string = "❯" // chat input prompt

	// Status / results
	CheckIcon   string = "✓"
	CrossIcon   string = "✕"
	ErrorIcon   string = "✖"
	WarningIcon string = "⚠"
	InfoIcon    string = "ℹ"
	HintIcon    string = "◇"

	// Motion
	SpinnerIcon string = "⟳"
	LoadingIcon string = "⋯"

	// Content
	DocumentIcon string = "▤" // attachments / files
	TerminalIcon string = "▮" // command / tool execution
)
