package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kocieusz/memex/internal/config"
	"github.com/kocieusz/memex/internal/target"
	"github.com/kocieusz/memex/internal/tui"
)

// providersCmd groups the provider subcommands. Every harness memex detects is
// managed unless it is disabled here, so there is nothing to opt into after
// installing one.
type providersCmd struct {
	Ls      providersLsCmd      `cmd:"" default:"1" help:"Show every supported provider and its status."`
	Enable  providersEnableCmd  `cmd:"" help:"Manage a provider that was disabled."`
	Disable providersDisableCmd `cmd:"" help:"Leave a provider alone, without unlinking what is there."`
}

type providersLsCmd struct{}

func (p *providersLsCmd) Run(c *cli) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	all := target.All()
	nameW := 0
	for _, b := range all {
		nameW = max(nameW, len(b.Name))
	}
	for _, b := range all {
		tone, state := tui.Good, "enabled"
		switch {
		case slices.Contains(cfg.DisabledProviders, b.Name):
			tone, state = tui.Muted, "disabled"
		case !b.Present():
			tone, state = tui.Muted, "missing"
		}
		fmt.Printf("%s %-*s %s\n", tui.Badge(tone, state), nameW, b.Name, tui.Dim(shortHome(b.Path)))
	}
	if len(target.Builtins()) < len(all) {
		fmt.Println()
		fmt.Println(tui.Dim("missing providers are not installed on this machine; memex picks them up automatically once they are"))
	}
	return nil
}

type providersEnableCmd struct {
	Name string `arg:"" help:"Provider to manage again (claude, codex, pi, agents)."`
}

func (p *providersEnableCmd) Run(c *cli) error {
	cfg, err := loadProviderConfig(p.Name)
	if err != nil {
		return err
	}
	if !slices.Contains(cfg.DisabledProviders, p.Name) {
		fmt.Printf("%s is already enabled\n", p.Name)
		return nil
	}
	rest := slices.DeleteFunc(slices.Clone(cfg.DisabledProviders), func(n string) bool { return n == p.Name })
	if err := config.SetDisabledProviders(rest); err != nil {
		return err
	}
	fmt.Printf("enabled %s\n", p.Name)
	if !providerPresent(p.Name) {
		fmt.Printf("note: %s isn't installed yet — memex will start linking into it once it is\n", p.Name)
		return nil
	}
	fmt.Println("run `memex sync` to link the current set into it")
	return nil
}

type providersDisableCmd struct {
	Name string `arg:"" help:"Provider to stop managing (claude, codex, pi, agents)."`
}

func (p *providersDisableCmd) Run(c *cli) error {
	cfg, err := loadProviderConfig(p.Name)
	if err != nil {
		return err
	}
	if slices.Contains(cfg.DisabledProviders, p.Name) {
		fmt.Printf("%s is already disabled\n", p.Name)
		return nil
	}
	disabled := append(slices.Clone(cfg.DisabledProviders), p.Name)
	slices.Sort(disabled)
	if err := config.SetDisabledProviders(disabled); err != nil {
		return err
	}
	path, err := config.Path()
	if err != nil {
		return err
	}
	fmt.Printf("disabled %s in %s\n", p.Name, shortHome(path))
	fmt.Println(tui.Dim("links already there are left as they are; remove them with `memex providers enable " + p.Name + "` and a `memex` apply, or by hand"))
	return nil
}

// loadProviderConfig reads the config after checking that name is a provider
// memex knows, so a typo can't be written to the config file.
func loadProviderConfig(name string) (config.Config, error) {
	if !target.Supported(name) {
		return config.Config{}, fmt.Errorf("unknown provider %q: expected one of %s", name, strings.Join(target.Names(target.All()), ", "))
	}
	return config.Load()
}

func providerPresent(name string) bool {
	for _, b := range target.All() {
		if b.Name == name {
			return b.Present()
		}
	}
	return false
}
