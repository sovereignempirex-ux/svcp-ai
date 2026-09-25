package styles

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

func getColorRGB(c lipgloss.TerminalColor) (uint8, uint8, uint8) {
	r, g, b, a := c.RGBA()

	// Un-premultiply alpha if needed
	if a > 0 && a < 0xffff {
		r = (r * 0xffff) / a
		g = (g * 0xffff) / a
		b = (b * 0xffff) / a
	}

	// Convert from 16-bit to 8-bit color
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

// ForceReplaceBackgroundWithLipgloss stamps a 24‑bit background colour on
// every styled span of `input`.
//
// Spans that already carry an explicit background are left untouched: that is
// what lets deliberately highlighted blocks (code blocks, diffs) keep their own
// surface while everything else inherits the surrounding panel colour.
func ForceReplaceBackgroundWithLipgloss(input string, newBgColor lipgloss.TerminalColor) string {
	// Precompute our new-bg sequence once
	r, g, b := getColorRGB(newBgColor)
	newBg := fmt.Sprintf("48;2;%d;%d;%d", r, g, b)

	return ansiEscape.ReplaceAllStringFunc(input, func(seq string) string {
		const (
			escPrefixLen = 2 // "\x1b["
			escSuffixLen = 1 // "m"
		)

		raw := seq
		start := escPrefixLen
		end := len(raw) - escSuffixLen

		var sb strings.Builder
		// reserve enough space: original content minus bg codes + our newBg
		sb.Grow((end - start) + len(newBg) + 2)

		// Set when the sequence already defines its own background.
		hasBG := false

		// scan from start..end, token by token
		for i := start; i < end; {
			// find the next ';' or end
			j := i
			for j < end && raw[j] != ';' {
				j++
			}
			token := raw[i:j]

			// fast‑path: skip "48;5;N" or "48;2;R;G;B"
			if len(token) == 2 && token[0] == '4' && token[1] == '8' {
				k := j + 1
				if k < end {
					// find next token
					l := k
					for l < end && raw[l] != ';' {
						l++
					}
					next := raw[k:l]
					if next == "5" {
						// skip "48;5;N"
						m := l + 1
						for m < end && raw[m] != ';' {
							m++
						}
						i = m + 1
						hasBG = true
						continue
					} else if next == "2" {
						// skip "48;2;R;G;B"
						m := l + 1
						for count := 0; count < 3 && m < end; count++ {
							for m < end && raw[m] != ';' {
								m++
							}
							m++
						}
						i = m
						hasBG = true
						continue
					}
				}
			}

			// decide whether to keep this token
			// manually parse ASCII digits to int
			isNum := true
			val := 0
			for p := i; p < j; p++ {
				c := raw[p]
				if c < '0' || c > '9' {
					isNum = false
					break
				}
				val = val*10 + int(c-'0')
			}
			keep := !isNum ||
				((val < 40 || val > 47) && (val < 100 || val > 107) && val != 49)

			if keep {
				if sb.Len() > 0 {
					sb.WriteByte(';')
				}
				sb.WriteString(token)
			}
			// advance past this token (and the semicolon)
			i = j + 1
		}

		// An explicit background wins: keep the sequence exactly as it is.
		if hasBG {
			if sb.Len() == 0 {
				return raw
			}
			return "\x1b[" + sb.String() + "m"
		}

		// append our new background
		if sb.Len() > 0 {
			sb.WriteByte(';')
		}
		sb.WriteString(newBg)

		return "\x1b[" + sb.String() + "m"
	})
}
