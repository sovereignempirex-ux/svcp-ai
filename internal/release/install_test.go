package release

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"
)

// findRoot locates the repository by walking up from the working directory. A
// fixed relative path would be enough for `go test`, which runs a package in its
// own directory, but not for the compiled test binary run from the top of the
// tree, which is how these have to be run when the platform refuses to execute
// a freshly built one.
func findRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot read the working directory: %v", err)
	}
	for {
		// install has no extension and lives at the top level, so it identifies
		// the root as well as go.mod does.
		_, hasGoMod := os.Stat(filepath.Join(dir, "go.mod"))
		_, hasInstall := os.Stat(filepath.Join(dir, "install"))
		if hasGoMod == nil && hasInstall == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no repository root above %s; the checks here cannot run", dir)
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRoot(t), name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

// target is one thing the release produces.
type target struct {
	goos   string
	goarch string
}

// ── The release configuration ────────────────────────────────────

var (
	goosList   = regexp.MustCompile(`goos:\s*\[([^\]]*)\]`)
	goarchList = regexp.MustCompile(`goarch:\s*\[([^\]]*)\]`)
	// An ignored pair is written as a list item with a goos and a goarch.
	ignoredPair = regexp.MustCompile(`-\s*goos:\s*"?([\w.]+)"?\s*\n\s*goarch:\s*"?([\w.]+)"?`)
	// formatOverride redirects one platform to a different archive format.
	formatOverride = regexp.MustCompile(`goos:\s*"?([\w.]+)"?\s*\n\s*formats:\s*\[([^\]]*)\]`)
	formatList     = regexp.MustCompile(`formats:\s*\[([^\]]*)\]`)
)

func splitList(body string) []string {
	var out []string
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"' `)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// buildMatrix is the cross product of the declared operating systems and
// architectures, less the pairs the configuration ignores.
func buildMatrix(t *testing.T, config string) []target {
	t.Helper()

	osMatch := goosList.FindStringSubmatch(config)
	if osMatch == nil {
		t.Fatal("no goos list in the release configuration; the checks here cannot be trusted")
	}
	archMatch := goarchList.FindStringSubmatch(config)
	if archMatch == nil {
		t.Fatal("no goarch list in the release configuration; the checks here cannot be trusted")
	}

	skip := map[target]bool{}
	for _, pair := range ignoredPair.FindAllStringSubmatch(config, -1) {
		skip[target{pair[1], pair[2]}] = true
	}

	var out []target
	for _, goos := range splitList(osMatch[1]) {
		for _, goarch := range splitList(archMatch[1]) {
			if skip[target{goos, goarch}] {
				continue
			}
			out = append(out, target{goos, goarch})
		}
	}
	return out
}

// archiveFormats is the extension a platform's archive gets, after any override.
func archiveFormats(t *testing.T, config string, goos string) string {
	t.Helper()

	// The first formats list is the archives default; an override for this
	// platform replaces it.
	base := ""
	if m := formatList.FindStringSubmatch(config); m != nil {
		base = strings.TrimSpace(strings.SplitN(m[1], ",", 2)[0])
	}
	for _, over := range formatOverride.FindAllStringSubmatch(config, -1) {
		if over[1] == goos {
			return strings.TrimSpace(over[2])
		}
	}
	return base
}

// nameTemplate is the archive name template, folded the way YAML folds it, so
// that the value is byte for byte what goreleaser will execute.
//
// It reports false rather than guessing when the shape is not the one it knows.
// A configuration edit that moves or renames the key then fails the test instead
// of quietly disabling every check in this file.
func nameTemplate(t *testing.T, config string) (string, bool) {
	t.Helper()

	lines := strings.Split(config, "\n")
	start := -1
	indent := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name_template:") {
			start = i
			indent = len(line) - len(strings.TrimLeft(line, " "))
			break
		}
	}
	if start < 0 {
		return "", false
	}

	// The value is on the key's line when it is not a block scalar, and below it
	// when it is. Only the block form is handled; an inline value would need its
	// own quoting rules and is not a shape worth guessing at.
	key := strings.TrimSpace(lines[start])
	if !strings.HasSuffix(key, ">-") && !strings.HasSuffix(key, ">") {
		return "", false
	}

	var parts []string
	for _, line := range lines[start+1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// The block ends at the first line indented no further than the key.
		if len(line)-len(strings.TrimLeft(line, " ")) <= indent {
			break
		}
		parts = append(parts, strings.TrimSpace(line))
	}
	if len(parts) == 0 {
		return "", false
	}
	// A folded scalar joins its lines with a space; the template's own trim
	// markers remove it again where it matters.
	return strings.Join(parts, " "), true
}

// archiveName runs the real template, with the same fields goreleaser provides.
// text/template is what goreleaser uses, so a template that is malformed fails
// here the same way it would there.
func archiveName(t *testing.T, nameTmpl string, tj target) string {
	t.Helper()

	tmpl, err := template.New("archive").Parse(nameTmpl)
	if err != nil {
		t.Fatalf("the archive name template does not parse: %v\n%s", err, nameTmpl)
	}
	var out bytes.Buffer
	data := map[string]string{
		"ProjectName": "svpc",
		"Os":          tj.goos,
		"Arch":        tj.goarch,
		"Version":     "1.0.0",
	}
	if err := tmpl.Execute(&out, data); err != nil {
		t.Fatalf("the archive name template does not run for %s/%s: %v", tj.goos, tj.goarch, err)
	}
	return out.String()
}

// ── The install script ────────────────────────────────────────────

var (
	// An architecture arm, as written in the case statement.
	archArm = regexp.MustCompile(`([a-z0-9_|]+)\)\s*arch="([a-z0-9_]+)"`)
	// The only rename the script makes: the kernel's name for the OS, as a
	// single comparison and assignment.
	osRename = regexp.MustCompile(`\[\[ "\$os" == "([\w.]+)" \]\] && os="([\w]+)"`)
	// The architectures a platform accepts, quoted in the guard. The whole
	// condition is captured, because a platform may name more than one and
	// taking only the first would silently drop the others.
	archGuard = regexp.MustCompile(`([\w]+)\)\s*\[\[([^\]]*)\]\]`)
	// Every quoted word inside such a condition.
	quoted = regexp.MustCompile(`"([a-z0-9_]+)"`)
	// The archive name the script builds, which carries the application name and
	// the extension as well as the platform. The extension has more than one part
	// — ".tar.gz" — so every part of it is captured, not just the first.
	archiveLine = regexp.MustCompile(`(?m)^archive="\$APP-\$os-\$arch(\.[A-Za-z0-9.]+)"`)
	appName     = regexp.MustCompile(`(?m)^APP="?([A-Za-z0-9_.-]+)"?$`)
	// The extension the script assumes, taken from the tar it runs.
	tarFormat = regexp.MustCompile(`tar -xzf`)
)

// installerNames is every archive name the install script can ask for, and the
// extension it insists on.
func installerNames(t *testing.T, script string) (map[string]string, string) {
	t.Helper()

	if !tarFormat.MatchString(script) {
		t.Fatal("the install script no longer untars anything; the format assumption here is wrong")
	}
	app := appName.FindStringSubmatch(script)
	if app == nil {
		t.Fatal("the install script no longer names the application the way this check expects")
	}
	ext := archiveLine.FindStringSubmatch(script)
	if ext == nil {
		t.Fatal(`the install script no longer builds "$APP-$os-$arch<ext>" the way this check expects`)
	}

	renames := map[string]string{}
	for _, m := range osRename.FindAllStringSubmatch(script, -1) {
		renames[m[1]] = m[2]
	}
	if len(renames) != 1 {
		t.Fatalf("expected exactly one operating-system rename, found %d: %v",
			len(renames), renames)
	}
	var renamedFrom, renamedTo string
	for from, to := range renames {
		renamedFrom, renamedTo = from, to
	}

	// Every arm of the architecture case, and what it maps to.
	arch := map[string]string{}
	for _, m := range archArm.FindAllStringSubmatch(script, -1) {
		for _, spelling := range strings.Split(m[1], "|") {
			arch[strings.TrimSpace(spelling)] = m[2]
		}
	}
	if len(arch) == 0 {
		t.Fatal("no architecture mapping found in the install script")
	}

	// A platform is served if the script reaches the download step for it, which
	// means its name was not rejected above. The guard names the architectures
	// that a platform accepts; the one with no guard accepts them all.
	guards := map[string][]string{}
	for _, m := range archGuard.FindAllStringSubmatch(script, -1) {
		for _, arch := range quoted.FindAllStringSubmatch(m[2], -1) {
			guards[m[1]] = append(guards[m[1]], arch[1])
		}
	}

	names := map[string]string{}
	for _, spelling := range sortedKeys(arch) {
		// The kernel name is renamed in one direction only, so a platform with no
		// rename keeps its own name.
		for _, platform := range []string{renamedFrom, renamedTo, "linux"} {
			allowed, gated := guards[platform]
			if gated && !contains(allowed, arch[spelling]) {
				continue
			}
			// The rename applies to the platform it names; the other keeps the
			// kernel's own name, which is the same string.
			name := platform
			if platform == renamedFrom {
				name = renamedTo
			}
			names[app[1]+"-"+name+"-"+arch[spelling]+ext[1]] = strings.TrimPrefix(ext[1], ".")
		}
	}
	return names, strings.TrimPrefix(ext[1], ".")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Insertion order would make the test output jump around between runs.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// ── The checks ────────────────────────────────────────────────────

// The name template has to be found, or none of the name checks mean anything.
// Said first so a configuration edit is reported as itself rather than as a
// missing archive.
func TestTheNameTemplateIsWhereItIsExpected(t *testing.T) {
	nameTmpl, ok := nameTemplate(t, readRepoFile(t, ".goreleaser.yml"))
	if !ok {
		t.Fatal(".goreleaser.yml no longer has a folded archives name_template where it was")
	}
	if !strings.Contains(nameTmpl, ".ProjectName") {
		t.Errorf("the name template ignores the project name: %s", nameTmpl)
	}
	// Every name must be built from the project name and the platform, or a
	// release would overwrite the last one.
	if strings.Count(nameTmpl, ".ProjectName") != 1 {
		t.Errorf("the name template should use the project name exactly once: %s", nameTmpl)
	}
}

// Every archive the release produces, named the way the release names it, with
// the extension the configuration gives it.
func releasedArchives(t *testing.T) map[string]string {
	t.Helper()

	config := readRepoFile(t, ".goreleaser.yml")
	nameTmpl, ok := nameTemplate(t, config)
	if !ok {
		t.Fatal(".goreleaser.yml no longer has a folded archives name_template where it was")
	}

	out := map[string]string{}
	for _, tj := range buildMatrix(t, config) {
		format := archiveFormats(t, config, tj.goos)
		if format == "" {
			t.Fatalf("no archive format for %s/%s", tj.goos, tj.goarch)
		}
		// The template carries no extension; goreleaser appends the format, so
		// the two have to be joined here to get the name a user would download.
		out[archiveName(t, nameTmpl, tj)+"."+format] = format
	}
	return out
}

// The install script has to be able to fetch everything the release produces.
// A platform that is built but not installable is the worst kind of gap: the
// release page offers it, the installer asks for it, and the answer is a 404.
func TestEveryReleasedArchiveCanBeInstalled(t *testing.T) {
	installable, _ := installerNames(t, readRepoFile(t, "install"))
	released := releasedArchives(t)

	if len(released) == 0 {
		t.Fatal("no archives are described; the configuration was not understood")
	}

	// Windows is served by the signed executable that the release attaches
	// separately, not by the install script, so it is out of scope here.
	for name, format := range released {
		if strings.HasPrefix(name, "svpc-windows") {
			continue
		}
		want, ok := installable[name]
		if !ok {
			t.Errorf("the release produces %s but the install script has no name for it", name)
			continue
		}
		if want != format {
			t.Errorf("%s is released as %s but the installer expects %s", name, format, want)
		}
	}
}

// The reverse: a name the installer will ask for, with nothing behind it. A
// platform the script accepts but the release never builds means a user on that
// hardware is told the installer supports them, and then gets a download failure.
func TestTheInstallerPromisesNothingItCannotDeliver(t *testing.T) {
	installable, _ := installerNames(t, readRepoFile(t, "install"))
	released := releasedArchives(t)

	for _, name := range sortedKeys(installable) {
		if _, ok := released[name]; !ok {
			t.Errorf("the install script offers %s, which the release does not produce\n"+
				"        released:   %s\n"+
				"        installable: %s",
				name, strings.Join(sortedKeys(released), " "), strings.Join(sortedKeys(installable), " "))
		}
	}
}

// The install script only knows how to unpack a gzipped tar. A platform that
// ships as anything else is unreachable by every user on it.
func TestEveryInstallableArchiveIsATarball(t *testing.T) {
	released := releasedArchives(t)
	installable, assumed := installerNames(t, readRepoFile(t, "install"))

	for name, format := range released {
		if _, ok := installable[name]; !ok {
			continue
		}
		if format != assumed {
			t.Errorf("%s is released as a %s, which the install script cannot unpack", name, format)
		}
	}
}

// Whatever has actually been built has to be named the way the configuration
// says. This catches a hand-renamed file before it is published rather than
// after, and is skipped when nothing has been built, which is the normal state
// of a fresh checkout.
func TestBuiltArchivesAreNamedAsConfigured(t *testing.T) {
	dist := filepath.Join(findRoot(t), "dist")
	entries, err := os.ReadDir(dist)
	if err != nil {
		t.Skipf("nothing built yet: %v", err)
	}

	released := releasedArchives(t)
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		checked++
		if _, ok := released[e.Name()]; !ok {
			keys := sortedKeys(released)
			t.Errorf("dist/%s is not a name the release configuration produces; it produces %s",
				e.Name(), strings.Join(keys, ", "))
		}
	}
	if checked == 0 {
		t.Skip("dist/ holds no archives yet")
	}
}

// The published checksums name the files as GitHub will store them, so a name
// with a space in it is written down differently from the one on disk. Getting
// that wrong makes every verification fail on a file that is perfectly fine.
func TestChecksumNamesAreTheOnesGitHubWillUse(t *testing.T) {
	script := readRepoFile(t, "scripts/publish-release.ps1")

	// The rewriting rule, taken from the script rather than restated, so a
	// change to the script is a change to what is expected here.
	sanitise := regexp.MustCompile(`\$name\s*=\s*\(\s*(?:Get-UploadName[^)]*)\)`)
	if !sanitise.MatchString(script) {
		t.Fatal("the publish script no longer rewrites asset names the way this check assumes")
	}
	if !strings.Contains(script, "Get-UploadName") {
		t.Fatal("the publish script has no Get-UploadName helper")
	}

	// What GitHub does: anything outside this set becomes a dot.
	keep := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		default:
			return '.'
		}
	}
	// The executable is the one asset with a space in its name, so it is the one
	// that can go wrong.
	const exe = "SVPC AI.exe"
	got := strings.Map(keep, exe)
	if got != "SVPC.AI.exe" {
		t.Errorf("the executable is published as %q; the release refers to it as SVPC.AI.exe", got)
	}
	// And the archive names must be untouched, since they are already safe.
	for name := range releasedArchives(t) {
		if mapped := strings.Map(keep, name); mapped != name {
			t.Errorf("%s would be published as %s, so the installer would not find it", name, mapped)
		}
	}
}
