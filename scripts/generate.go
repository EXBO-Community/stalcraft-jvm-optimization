package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"go/format"

	"golang.org/x/mod/semver"
)

var (
	tagRe    = regexp.MustCompile(`v\d+\.\d+\.\d+`)
	oldGenRe = regexp.MustCompile(`func\s*(?:\([^)]+\))?\s+generateV(\d+)([A-Z][a-zA-Z_]*)\([^)]*\)`)
	newGenRe = regexp.MustCompile(`func\s*(?:\([^)]+\))?\s+(\w+)_generate([A-Z][a-zA-Z_]*)\([^)]*\)`)
	rootDir  = findProjectRoot()
	releases map[string]*textRelease
	tags     []string
)

type textPreset struct {
	name     string
	funcName string
	desc     string
}

type textRelease struct {
	desc    string // now same with default preset's description, maybe will be changed later
	presets []textPreset
}

func main() {
	if err := generateBuildinfoGo(); err != nil {
		slog.Error("Failed to generate buildinfo", "error", err)
		os.Exit(1)
	}
	slog.Info("Successfully generated internal/buildinfo/buildinfo_generated.go")
	cmd := exec.Command(
		"git",
		"tag",
		"-l",
		"--sort=-v:refname",
		"v[0-9]*.[0-9]*.[0-9]*",
	)
	output, err := cmd.Output()
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to get tag list, ensure you have them, see file:///%s", filepath.Join(rootDir, "docs", "OVERVIEW.md")), "error", err)
		os.Exit(1)
	}
	out := strings.Split(strings.TrimSpace(string(output)), "\n")
	versionToTag := make(map[string]string, len(out))
	releases = make(map[string]*textRelease, len(out))
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
		if tagRe.MatchString(out[i]) {
			versionToTag[tagToVersion(out[i])] = out[i]
		}
	}
	files, _ := filepath.Glob(filepath.Join(rootDir, "internal", "profile", "*.go"))
	for i := range files {
		filename := strings.TrimSuffix(filepath.Base(files[i]), filepath.Ext(files[i]))
		if filename == "release" || filename == "release_generated" {
			continue
		}
		data, err := os.ReadFile(files[i])
		if err != nil {
			slog.Error("Failed to read file specified", "error", files[i])
			os.Exit(1)
		}
		file_tag := findFirstTag(files[i])
		var tag, preset, funcName, comment string
		for j, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") { // have eol=lf for .go, but to ensure
			if strings.HasPrefix(line, "// ") {
				comment = strings.TrimPrefix(line, "// ")
				continue
			}
			matches := oldGenRe.FindStringSubmatch(line)
			if len(matches) >= 3 && matches[2] == "Default" {
				if val, ok := versionToTag[matches[1]]; ok {
					tag = val
				} else {
					slog.Error("Unknown version in generator", "error", fmt.Sprintf("concatenated tag: \"%s\". File: %s, line %d", matches[1], filename, j+1))
					os.Exit(1)
				}
				preset = "default"
				funcName = fmt.Sprintf("generateV%sDefault", matches[1])
			} else { // in fact didn't find any examples of matching both regexes
				matches = newGenRe.FindStringSubmatch(line)
				if len(matches) >= 3 && matches[1] == filename {
					preset = strings.ToLower(matches[2])
					tag = file_tag
					funcName = fmt.Sprintf("%s_generate%s", filename, matches[2])
				} else {
					comment = ""
					continue
				}
			}
			slog.Info(fmt.Sprintf("Parsed preset: %s/%s; %s\n%s", tag, preset, funcName, comment))
			if _, ok := releases[tag]; !ok {
				tags = append(tags, tag)
				releases[tag] = &textRelease{}
			}
			if preset == "default" { // default must be
				releases[tag].desc = comment
			}
			releases[tag].presets = append(releases[tag].presets, textPreset{name: preset, funcName: funcName, desc: comment})
		}
		slog.Info("Successfully parsed file internal/profile/" + filename + ".go")
	}
	if err := generateReleaseGo(); err != nil {
		slog.Error("Failed to generate releases", "error", err)
		os.Exit(1)
	}
	slog.Info("Successfully generated internal/profile/releases_generated.go")
}

func tagToVersion(tag string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, tag)
}

func findFirstTag(filePath string) string {
	// Get commit hash that added the file
	cmd := exec.Command("git", "log", "--diff-filter=A", "--format=%H", "--", filePath)
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return "vnew" // not commited
	}
	commit := strings.TrimSpace(string(output))
	if commit == "" {
		return "vnew"
	}
	cmd = exec.Command("git", "tag", "--contains", commit, "--sort=creatordate")
	output, err = cmd.Output()
	if err != nil || len(output) == 0 {
		return "vnew" // ahead the last tag
	}
	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(tags) == 0 {
		return "vnew"
	}
	if tags[0][0] != 'v' { // can be "undefined"
		tags[0] = "v" + tags[0]
	}
	return tags[0]
}

func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		slog.Error("Failed to find project root", "error", err)
		os.Exit(1)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			slog.Error("Failed to find project root", "error", "go.mod not found")
			os.Exit(1)
		}
		dir = parent
	}
}

func generateBuildinfoGo() error {
	cmd := exec.Command(
		"git",
		"describe",
		"--tags",
		"--always",
		"--dirty",
	)
	version, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get git version: %w", err)
	}
	cmd = exec.Command(
		"git",
		"rev-parse",
		"--short",
		"HEAD",
	)
	commit, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get git commit: %w", err)
	}
	tmpl := `// This code is generated by scripts/generate.go; DO NOT EDIT if not sure
package buildinfo

func init() {
	Version = "{{.Version}}"
	Commit = "{{.Commit}}"
}
`
	f, err := os.Create(filepath.Join(rootDir, "internal", "buildinfo", "buildinfo_generated.go"))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()
	data := struct {
		Version string
		Commit  string
	}{
		Version: strings.TrimSpace(string(version)),
		Commit:  strings.TrimSpace(string(commit)),
	}
	t := template.Must(template.New("buildinfo").Parse(tmpl))
	if err := t.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	return nil
}

func generateReleaseGo() error {
	slices.SortFunc(tags, func(a, b string) int {
		return semver.Compare(a, b)
	})
	var buf bytes.Buffer
	buf.Grow(len(releases) * 100)
	buf.WriteString(`// This code is generated by scripts/generate.go; DO NOT EDIT if not sure
package profile

func init() {
	releases = []Release{
	`)
	for _, tag := range tags {
		r := releases[tag]
		presets := ""
		for _, preset := range r.presets {
			presets += fmt.Sprintf("\t\t\t\t{Name: \"%s\", Label: \"%s\", Description: \"%s\", Generate: %s},\n", preset.name, preset.name, preset.desc, preset.funcName)
		}
		fmt.Fprintf(&buf, `	{
			Version: "%s",
			Label: "%s",
			Description: "%s",
			Presets: []Preset{
	%s			},
		},
	`, tag, tag, r.desc, presets)
	}
	buf.WriteString("\t\n}\n}")
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated code: %w", err)
	}
	if err = os.WriteFile(filepath.Join(rootDir, "internal", "profile", "releases_generated.go"), formatted, 0644); err != nil {
		return fmt.Errorf("failed to write to internal/profile/releases_generated.go: %w", err)
	}
	return nil
}
