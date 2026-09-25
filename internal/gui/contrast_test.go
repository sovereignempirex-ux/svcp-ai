package gui

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The window's palette is checked against WCAG 2.1 here rather than by eye.
//
// A colour change is easy to make and easy to get wrong: nudging an accent for
// looks can quietly drop a label below the readable threshold, and nothing else
// in the build would notice. Each entry names the pair and the surface it
// appears on, so a failure points at something that can be looked at.

// contrastPair is one foreground/background combination that has to be legible.
type contrastPair struct {
	fg, bg string
	// level is the minimum ratio: 4.5 for body text, 3 for large text and for
	// the non-text parts of a control (WCAG 1.4.11).
	level float64
	where string
}

// contrastPairs are the combinations the window actually renders.
//
// Divider colours are excluded on purpose: they separate content but carry no
// meaning alone, and raising them to a visible level would only add noise.
var contrastPairs = []contrastPair{
	{"--text", "--canvas", 4.5, "message text on the page"},
	{"--text", "--surface", 4.5, "message text on a card"},
	{"--text", "--surface-hi", 4.5, "text on a raised control"},
	{"--text", "--code-bg", 4.5, "text in a code block"},

	{"--text-muted", "--canvas", 4.5, "captions on the page"},
	{"--text-muted", "--surface", 4.5, "sidebar stat labels"},
	{"--text-muted", "--surface-hi", 4.5, "input placeholders"},
	{"--text-muted", "--code-bg", 4.5, "muted text in a code block"},
	{"--text-emph", "--surface", 4.5, "the wordmark"},

	{"--primary", "--canvas", 4.5, "the running-tool label"},
	{"--warning", "--canvas", 4.5, "the setup banner heading"},
	{"--error", "--canvas", 4.5, "error text"},
	{"--success", "--canvas", 4.5, "the completed tool tick"},
	{"--diff-add", "--code-bg", 4.5, "added lines in a diff"},

	// Text on a filled control.
	{"--on-accent", "--primary", 4.5, "the Send button label"},
	{"--on-success", "--success", 4.5, "the Allow button label"},

	// A control boundary has to be findable, so it clears 3:1 on every surface
	// it can sit on. One value serves all three, which is why it is solved
	// against the lightest of them.
	{"--control-border", "--canvas", 3, "the composer and input borders"},
	{"--control-border", "--surface", 3, "a field border on a panel"},
	{"--control-border", "--surface-hi", 3, "a field border on a raised surface"},
}

// TestPaletteContrastHolds checks both the default and the high-contrast
// palettes. The high-contrast variant is the one that exists to be readable, so
// it is held to the same bar rather than assumed better.
func TestPaletteContrastHolds(t *testing.T) {
	css := asset(t, "app.css")

	palettes := map[string]map[string]string{
		"default":  paletteTokens(css, ":root"),
		"contrast": paletteTokens(css, "body.contrast"),
	}

	names := make([]string, 0, len(palettes))
	for name := range palettes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		// The high-contrast variant only overrides a handful of tokens, so it
		// is layered over the default palette the way a browser would resolve
		// it, rather than read as a palette in its own right.
		tokens := map[string]string{}
		for k, v := range paletteTokens(css, ":root") {
			tokens[k] = v
		}
		for k, v := range palettes[name] {
			tokens[k] = v
		}

		t.Run(name, func(t *testing.T) {
			var failures []string
			for _, p := range contrastPairs {
				fg, bg := tokens[p.fg], tokens[p.bg]
				if fg == "" || bg == "" {
					failures = append(failures,
						fmt.Sprintf("%s: %s or %s is not defined", p.where, p.fg, p.bg))
					continue
				}

				got := contrastRatio(fg, bg)
				if got+0.005 < p.level {
					failures = append(failures, fmt.Sprintf(
						"%s: %s on %s is %.2f:1, needs %.1f:1",
						p.where, p.fg, p.bg, got, p.level))
				}
			}

			if len(failures) > 0 {
				t.Errorf("%d pair(s) below target:\n  %s",
					len(failures), strings.Join(failures, "\n  "))
			}
		})
	}
}

// paletteTokens reads the custom properties declared by one selector block.
//
// Only hex values are collected: a token that computes its own colour is
// reported as missing rather than silently skipped, because the checks below
// need a value to measure.
func paletteTokens(css, selector string) map[string]string {
	start := strings.Index(css, selector)
	if start < 0 {
		return nil
	}

	open := strings.Index(css[start:], "{")
	if open < 0 {
		return nil
	}
	open += start

	// Walk to the matching brace so a nested rule cannot end the block early.
	depth := 0
	end := -1
	for i := open; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil
	}

	block := css[open+1 : end]
	tokens := map[string]string{}
	for _, m := range regexp.MustCompile(`(--[\w-]+)\s*:\s*(#[0-9A-Fa-f]{3,8})\s*;`).FindAllStringSubmatch(block, -1) {
		tokens[m[1]] = m[2]
	}
	return tokens
}

// contrastRatio is the WCAG 2.1 contrast ratio between two hex colours, from 1
// (identical) to 21 (black on white).
func contrastRatio(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance implements the WCAG definition.
func relativeLuminance(hex string) float64 {
	r, g, b := parseHex(hex)
	lin := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// parseHex decodes #rgb or #rrggbb into normalised channels.
func parseHex(hex string) (float64, float64, float64) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) == 3 {
		h = strings.Join([]string{h[:1], h[:1], h[1:2], h[1:2], h[2:3], h[2:3]}, "")
	}
	if len(h) != 6 {
		return 0, 0, 0
	}
	v := func(i int) float64 {
		n, err := strconv.ParseUint(h[i:i+2], 16, 8)
		if err != nil {
			return 0
		}
		return float64(n) / 255
	}
	return v(0), v(2), v(4)
}

// TestColoursComeFromTokens keeps the palette honest.
//
// Every colour has to come from a token, so a component cannot quietly grow its
// own shade. The token blocks are the only places a hex is allowed.
func TestColoursComeFromTokens(t *testing.T) {
	css := asset(t, "app.css")

	// A hex in a colour or background declaration, outside a --token line.
	offender := regexp.MustCompile(`(?m)^\s*(?:color|background(?:-color)?|border(?:-\w+)?)\s*:[^;]*#[0-9A-Fa-f]{3,8}`)

	var offenders []string
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue // a token declaration, which is where a hex belongs
		}
		if offender.MatchString(line) {
			offenders = append(offenders, strings.TrimSpace(line))
		}
	}

	if len(offenders) > 0 {
		t.Errorf("these colours bypass the design tokens:\n  %s\n  declare one in :root instead",
			strings.Join(offenders, "\n  "))
	}
}

// TestPaletteHasNoUnusedTokens catches a token left behind once its last use
// went away, which is how a stylesheet quietly accumulates dead values.
func TestPaletteHasNoUnusedTokens(t *testing.T) {
	css := asset(t, "app.css")

	// Only the colour and type tokens, because the spacing and radius scales
	// are also reached through the shorthand rules.
	interesting := regexp.MustCompile(
		`--((?:[\w-]*(?:text|line|border|ink|accent|primary|success|error|warning|diff|mono|sans|hover|canvas|surface|code)[\w-]*))`)

	declared := map[string]bool{}
	for _, m := range interesting.FindAllStringSubmatch(css, -1) {
		declared[m[1]] = true
	}

	unused := []string{}
	for name := range declared {
		if !regexp.MustCompile(`var\(--` + regexp.QuoteMeta(name) + `[,)]`).MatchString(css) {
			unused = append(unused, "--"+name)
		}
	}
	sort.Strings(unused)

	if len(unused) > 0 {
		t.Errorf("declared but never used: %s", strings.Join(unused, ", "))
	}
}
