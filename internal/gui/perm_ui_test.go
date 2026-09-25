package gui

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// asset reads a file from the embedded front-end.
func asset(t *testing.T, name string) string {
	t.Helper()
	data, err := assets.ReadFile("assets/" + name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(data)
}

// assetPath resolves a path next to this source file, so a compiled test
// binary works from any working directory.
func assetPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test source file")
	}
	return filepath.Join(append([]string{filepath.Dir(thisFile)}, parts...)...)
}

// TestPermissionCardIsWiredUp checks that the approval card, the endpoint it
// posts to and the styles it uses all agree with each other.
//
// The card is the only part of the window a person touches while the agent is
// holding a decision open, so a mismatch here is a dead button rather than a
// cosmetic problem. These are static checks so they need no browser and no
// JavaScript runtime; testdata/permcheck.js drives the behaviour itself.
func TestPermissionCardIsWiredUp(t *testing.T) {
	js := asset(t, "app.js")
	css := asset(t, "app.css")

	t.Run("the stream handler routes permission events", func(t *testing.T) {
		// Without this branch the event is parsed and then dropped, and the
		// agent waits for an approval the window never shows.
		if !strings.Contains(js, "payload.permission") {
			t.Error("app.js does not handle the permission event")
		}
		if !strings.Contains(js, "showPermission(payload.permission)") {
			t.Error("the permission event is not passed to showPermission")
		}
	})

	t.Run("the card posts to the endpoint the server serves", func(t *testing.T) {
		post, _ := regexp.Compile(`fetch\(\s*["'](/api/[^"']+)["']\s*,\s*\{[^}]*method:\s*["']POST["']`)
		matches := post.FindAllStringSubmatch(js, -1)
		if len(matches) == 0 {
			t.Fatal("app.js makes no POST to an api endpoint")
		}

		served := map[string]bool{
			"/api/chat": true, "/api/permission": true,
		}
		found := false
		for _, m := range matches {
			if m[1] == "/api/permission" {
				found = true
			}
			if !served[m[1]] {
				t.Errorf("app.js posts to %q, which the server does not serve", m[1])
			}
		}
		if !found {
			t.Error("the approval card never posts to /api/permission")
		}
	})

	t.Run("every answer the server accepts is offered", func(t *testing.T) {
		for _, action := range []string{
			PermissionAllow, PermissionAllowForSession, PermissionDeny,
		} {
			if !strings.Contains(js, `"`+action+`"`) {
				t.Errorf("the card does not offer the %q answer", action)
			}
		}
	})

	t.Run("the card sends both fields the handler requires", func(t *testing.T) {
		// The handler rejects a response with no id and ignores an unknown one,
		// so both fields have to be in the posted body. Scoped to the object
		// literal that names both, so the unrelated config body cannot match.
		body := regexp.MustCompile(`JSON\.stringify\(\{[^}]*\bid\b[^}]*\baction\b[^}]*\}\)`).FindString(js)
		if body == "" {
			t.Fatal("the card does not post an object carrying both an id and an action")
		}
	})

	t.Run("every class the card uses is styled", func(t *testing.T) {
		used := regexp.MustCompile(`"(perm[\w-]*)"`).FindAllStringSubmatch(js, -1)
		if len(used) == 0 {
			t.Fatal("found no perm- classes in app.js")
		}
		styled := regexp.MustCompile(`(?m)\.([\w-]+)`).FindAllStringSubmatch(css, -1)
		defined := map[string]bool{}
		for _, m := range styled {
			defined[m[1]] = true
		}

		seen := map[string]bool{}
		for _, m := range used {
			name := m[1]
			if seen[name] {
				continue
			}
			seen[name] = true
			if !defined[name] {
				t.Errorf("app.js uses .%s but app.css does not define it", name)
			}
		}
	})

	t.Run("the three buttons have distinct styling", func(t *testing.T) {
		for _, variant := range []string{"btn-allow", "btn-always", "btn-deny"} {
			if !regexp.MustCompile(`(?m)^\.` + variant + `\s`).MatchString(css) {
				t.Errorf(".%s is used by the card but not styled", variant)
			}
		}
	})

	t.Run("every css variable resolves", func(t *testing.T) {
		declared := map[string]bool{}
		for _, m := range regexp.MustCompile(`(--[\w-]+)\s*:`).FindAllStringSubmatch(css, -1) {
			declared[m[1]] = true
		}
		for _, m := range regexp.MustCompile(`var\((--[\w-]+)\)`).FindAllStringSubmatch(css, -1) {
			// A variable defined only in a light-theme override still counts.
			if !declared[m[1]] {
				t.Errorf("app.css uses var(%s) but never defines it", m[1])
			}
		}
	})

	t.Run("the stylesheet is balanced", func(t *testing.T) {
		if open, close := strings.Count(css, "{"), strings.Count(css, "}"); open != close {
			t.Errorf("unbalanced braces: %d open, %d close", open, close)
		}
	})
}

// TestPermissionCheckScriptIsShippable guards against the behavioural check
// drifting back into the embedded assets, where it would be served to users.
func TestPermissionCheckScriptIsShippable(t *testing.T) {
	script := assetPath(t, "testdata", "permcheck.js")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("the permission check script is missing: %v", err)
	}

	// The embed directive takes everything under assets, so a stray script in
	// there would ship inside the binary.
	if _, err := assets.ReadFile("assets/permcheck.js"); err == nil {
		t.Error("permcheck.js is inside the embedded assets and would ship to users")
	}
}
