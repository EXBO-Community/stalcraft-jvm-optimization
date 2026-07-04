// Package profile contains frozen config generators for released tuning
// profiles. It deliberately depends on config storage, while config does not
// know anything about profile versions.
package profile

import (
	"fmt"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/config"
	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/sysinfo"
)

type Preset struct {
	Name        string
	Label       string
	Description string
	Generate    func(sysinfo.Info) config.Config
}

type Release struct {
	Version       string
	Label         string
	Description   string
	DefaultPreset string
	Presets       []Preset
}

type Generated struct {
	ID     string
	Config config.Config
}

var releases = []Release{
	release("v1.0.4", "Legacy runtime flags. 6GB minimum heap, high free-RAM based cap; rough on 16GB systems.", generateV104Default),
	release("v1.0.5", "Same tuning as v1.0.4, kept as a compatibility snapshot.", generateV105Default),
	release("v1.0.6", "Safer legacy heap sizing: requires 6GB free RAM and caps heap at 8GB.", generateV106Default),
	release("v1.0.7", "First generated JSON profile. Strong-CPU branch, 6GB minimum heap.", generateV107Default),
	release("v1.0.8", "Lower 4GB minimum heap and safer standard JIT settings.", generateV108Default),
	release("v1.1.0", "Modern cache-aware generator. More aggressive profile, up to 8GB heap.", generateV110Default),
	release("v1.1.1", "Memory-tier profile with calmer G1 pauses and a 6GB heap cap.", generateV111Default),
	release("v1.1.2", "Current stable v1.1 profile. Same generator as v1.1.1.", generateV112Default),
}

func release(version, description string, generate func(sysinfo.Info) config.Config) Release {
	return Release{
		Version:       version,
		Label:         version,
		Description:   description,
		DefaultPreset: "default",
		Presets: []Preset{
			{
				Name:     "default",
				Label:    "default",
				Generate: generate,
			},
		},
	}
}

func Releases() []Release {
	out := make([]Release, len(releases))
	copy(out, releases)
	return out
}

func Latest() Release {
	return releases[len(releases)-1]
}

func LatestDefaultID() string {
	return Latest().DefaultID()
}

func Find(version string) (Release, bool) {
	for _, r := range releases {
		if r.Version == version {
			return r, true
		}
	}
	return Release{}, false
}

func (r Release) DefaultID() string {
	return r.ID(r.DefaultPreset)
}

func (r Release) ID(preset string) string {
	return r.Version + "/" + preset
}

func (r Release) GenerateAll(sys sysinfo.Info) []Generated {
	out := make([]Generated, 0, len(r.Presets))
	for _, p := range r.Presets {
		out = append(out, Generated{
			ID:     r.ID(p.Name),
			Config: p.Generate(sys),
		})
	}
	return out
}

// Ensure creates the latest generated release if it is missing and chooses its
// default preset for first-time users. Existing active selections are left
// untouched, including legacy flat configs such as "default".
func Ensure(sys sysinfo.Info) error {
	if err := config.EnsureDir(); err != nil {
		return err
	}
	latest := Latest()
	for _, g := range latest.GenerateAll(sys) {
		if config.Exists(g.ID) {
			continue
		}
		if err := g.Config.Save(g.ID); err != nil {
			return err
		}
	}
	if config.ActiveName() == "" {
		if err := config.SetActive(latest.DefaultID()); err != nil {
			return err
		}
	}
	return nil
}

func Regenerate(version string, sys sysinfo.Info) ([]Generated, error) {
	r, ok := Find(version)
	if !ok {
		return nil, fmt.Errorf("profile release not found: %s", version)
	}
	generated := r.GenerateAll(sys)
	for _, g := range generated {
		if err := g.Config.Save(g.ID); err != nil {
			return nil, err
		}
	}
	if err := config.SetActive(r.DefaultID()); err != nil {
		return nil, err
	}
	return generated, nil
}
