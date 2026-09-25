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
// own shade. The rule is deliberately blunt rather than clever: any hex or rgb()
// literal outside a custom-property declaration is an offender.
//
// A precise grammar over colour-bearing properties looked like the better tool
// and was worse. It anchored to the start of a line, and every rule here is
// written on one line, so it matched nothing and would have passed forever.
func TestColoursComeFromTokens(t *testing.T) {
	css := asset(t, "app.css")

	// Both notations count: a hex is a flat colour, and an rgb() with an alpha
	// is how a tint of one is written.
	literal := regexp.MustCompile(`#[0-9A-Fa-f]{3,8}\b|rgba?\s*\(`)

	var offenders []string
	for number, line := range strings.Split(css, "\n") {
		// A custom-property declaration is where a literal belongs.
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		if literal.MatchString(line) {
			offenders = append(offenders,
				fmt.Sprintf("line %d: %s", number+1, strings.TrimSpace(line)))
		}
	}

	if len(offenders) > 0 {
		t.Errorf("these colours bypass the design tokens:\n  %s\n  declare one in :root instead",
			strings.Join(offenders, "\n  "))
	}
}

// inlineColours lists the colour literals written outside a token declaration.
func inlineColours(css string) []string {
	literal := regexp.MustCompile(`#[0-9A-Fa-f]{3,8}\b|rgba?\s*\(`)

	var found []string
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		if literal.MatchString(line) {
			found = append(found, strings.TrimSpace(line))
		}
	}
	return found
}

// TestPaletteAssertionsHaveTeeth guards the checks above against going vacuous.
//
// A test that cannot fail is worse than no test, and the inline-colour check
// already spent a commit silently unable to see anything. Each mutation below is
// a change the stylesheet ought to be rejected for, so the check has to notice.
func TestPaletteAssertionsHaveTeeth(t *testing.T) {
	original := asset(t, "app.css")

	if base := inlineColours(original); len(base) != 0 {
		t.Fatalf("the stylesheet already has %d inline colour(s):\n  %s",
			len(base), strings.Join(base, "\n  "))
	}

	mutations := []struct {
		name   string
		mutate func(string) string
	}{
		{"a one-line rule writes its own shade",
			func(s string) string { return s + "\n.bubble { color: #ff00ff; }" }},
		{"a multi-line rule writes its own shade",
			func(s string) string { return s + "\n.bubble {\n  color: #ff00ff;\n}\n" }},
		{"a background is written inline",
			func(s string) string { return s + "\n.hero { background: #123456; }" }},
		{"a border is written inline",
			func(s string) string { return s + "\n.tool { border: 1px solid #abcdef; }" }},
		{"a tint is written as rgba",
			func(s string) string { return s + "\n.msg .avatar { background: rgba(76,194,255,.14); }" }},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			if got := inlineColours(m.mutate(original)); len(got) == 0 {
				t.Error("the inline-colour check did not notice")
			}
		})
	}
}

// TestPaletteHasNoUnusedTokens catches a token left behind once its last use
// went away, which is how a stylesheet quietly accumulates dead values.
//
// Every token is checked, not only the colours: a scale entry nothing reaches is
// as dead as a colour, and three of them were.
func TestPaletteHasNoUnusedTokens(t *testing.T) {
	css := asset(t, "app.css")

	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(--[\w-]+)\s*:`).FindAllStringSubmatch(css, -1) {
		declared[m[1]] = true
	}

	unused := []string{}
	for name := range declared {
		if !regexp.MustCompile(`var\(` + regexp.QuoteMeta(name) + `[,)]`).MatchString(css) {
			unused = append(unused, name)
		}
	}
	sort.Strings(unused)

	if len(unused) > 0 {
		t.Errorf("declared but never used: %s", strings.Join(unused, ", "))
	}
}

// TestEveryVariableIsDefined catches a var() with no matching declaration.
//
// The failure is silent: CSS resolves an unknown custom property to nothing, so
// a colour or a spacing step simply disappears rather than erroring. The
// high-contrast block is excluded because it inherits from :root.
func TestEveryVariableIsDefined(t *testing.T) {
	css := asset(t, "app.css")

	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(--[\w-]+)\s*:`).FindAllStringSubmatch(css, -1) {
		declared[m[1]] = true
	}

	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`var\((--[\w-]+)`).FindAllStringSubmatch(css, -1) {
		used[m[1]] = true
	}

	var undefined []string
	for name := range used {
		if !declared[name] {
			undefined = append(undefined, name)
		}
	}
	sort.Strings(undefined)

	if len(undefined) > 0 {
		t.Errorf("used but never declared: %s", strings.Join(undefined, ", "))
	}
}

// TestNoTokenIsDefinedInTermsOfItself catches a bulk edit that rewrites a token
// declaration along with its uses, leaving --x: var(--x). CSS resolves that to
// nothing, so the colour silently disappears instead of erroring.
//
// The comparison is done in Go rather than with a backreference, which Go's
// regexp engine does not have.
func TestNoTokenIsDefinedInTermsOfItself(t *testing.T) {
	css := asset(t, "app.css")

	declaration := regexp.MustCompile(`(--[\w-]+)\s*:\s*var\((--[\w-]+)\)`)

	var offenders []string
	for _, m := range declaration.FindAllStringSubmatch(css, -1) {
		if m[1] == m[2] {
			offenders = append(offenders, m[1])
		}
	}
	sort.Strings(offenders)

	if len(offenders) > 0 {
		t.Errorf("these resolve to themselves and will render as nothing: %s",
			strings.Join(offenders, ", "))
	}
}
