package spec

import (
	"fmt"
	"slices"
	"strings"
)

// Snapshot describes a versioned CLI. Collected help may be incomplete;
// reference specifications can also supply explicit argument and forwarding rules.
type Snapshot struct {
	Tool    string     `json:"tool"`
	Shell   string     `json:"shell,omitempty"`
	Version string     `json:"version"`
	OS      string     `json:"os"`
	Sources []Evidence `json:"sources,omitempty"`
	Root    Command    `json:"root"`
}

type Evidence struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	Detail    string `json:"detail,omitempty"`
}

type Command struct {
	// UsageName is the program name printed in help, which may differ from argv[0].
	UsageName string `json:"usage_name,omitempty"`
	// SubcommandFirst means every matching usage starts with a command placeholder
	// (possibly after options), rather than an ordinary positional argument.
	SubcommandFirst       bool       `json:"subcommand_first,omitempty"`
	FlagsAfterPositionals bool       `json:"flags_after_positionals,omitempty"`
	ShortFlagClusters     bool       `json:"short_flag_clusters,omitempty"`
	AliasesComplete       bool       `json:"aliases_complete,omitempty"`
	Name                  string     `json:"name,omitempty"`
	Aliases               []string   `json:"aliases,omitempty"`
	Commands              []Command  `json:"commands,omitempty"`
	Flags                 []Flag     `json:"flags,omitempty"`
	Sources               []Evidence `json:"sources,omitempty"`
	// Forms are alternative argument grammars. Nil means bounds are unknown.
	Forms                 []Form `json:"forms,omitempty"`
	StopFlagsAfterOperand bool   `json:"stop_flags_after_operand,omitempty"`
	FlagOrderUnknown      bool   `json:"flag_order_unknown,omitempty"`
	DerivedForms          bool   `json:"derived_forms,omitempty"`
	LiteralArguments      bool   `json:"literal_arguments,omitempty"`
	Closed                bool   `json:"closed,omitempty"`
}

// Form describes one valid combination of flags and positional argument bounds.
// A nil maximum explicitly allows any number of positional arguments.
type Form struct {
	MinArgs   int      `json:"min_args"`
	MaxArgs   *int     `json:"max_args,omitempty"`
	Required  []string `json:"required_flags,omitempty"`
	Forbidden []string `json:"forbidden_flags,omitempty"`
}

type Flag struct {
	Name  string `json:"name"`
	Value string `json:"value"` // none, required, optional-attached, optional-separated, optional, or unknown.
	// ValueCount applies to required flags; zero means one value.
	ValueCount int      `json:"value_count,omitempty"`
	Choices    []string `json:"choices,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	// Forward consumes a command and its argv, without executing either.
	Forward    bool   `json:"forward,omitempty"`
	Terminator string `json:"terminator,omitempty"`
}

func (s *Snapshot) Key() string { return TargetKey(s.Tool, s.Shell) }

// NUL cannot occur in an executable name, so shell and executable namespaces
// stay disjoint even when a real executable has a name such as bash:echo.
func TargetKey(tool, shell string) string {
	if shell != "" {
		return "\x00" + shell + ":" + tool
	}
	return tool
}

func (s *Snapshot) Validate() error {
	if s.Shell != "" && s.Shell != "bash" {
		return fmt.Errorf("unsupported shell dialect %q", s.Shell)
	}
	if !validName(s.Tool) || strings.TrimSpace(s.Version) == "" || s.Version == "latest" {
		return fmt.Errorf("tool and a concrete version are required")
	}
	if s.Root.Name != "" {
		return fmt.Errorf("root must not have a name")
	}
	if err := validateEvidence(s.Sources); err != nil {
		return err
	}
	return validateCommand(&s.Root, s.Tool)
}

func validateCommand(c *Command, path string) error {
	if c.LiteralArguments && (len(c.Flags) > 0 || len(c.Commands) > 0) {
		return fmt.Errorf("%s: literal arguments cannot have flags or subcommands", path)
	}
	if c.UsageName != "" && !validName(c.UsageName) {
		return fmt.Errorf("%s: invalid help program name", path)
	}
	aliases := make(map[string]bool)
	for _, alias := range c.Aliases {
		if !validName(alias) || aliases[alias] {
			return fmt.Errorf("%s: invalid or repeated alias %q", path, alias)
		}
		aliases[alias] = true
	}
	if err := validateEvidence(c.Sources); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	flags := make(map[string]bool)
	for _, flag := range c.Flags {
		if flag.Value != "none" && flag.Value != "required" && flag.Value != "optional-attached" && flag.Value != "optional-separated" && flag.Value != "optional" && flag.Value != "unknown" {
			return fmt.Errorf("%s: %s has invalid value mode %q", path, flag.Name, flag.Value)
		}
		if flag.ValueCount < 0 || (flag.ValueCount != 0 && flag.Value != "required") {
			return fmt.Errorf("%s: %s has an invalid value count", path, flag.Name)
		}
		for _, name := range append([]string{flag.Name}, flag.Aliases...) {
			if !validFlag(name) || flags[name] {
				return fmt.Errorf("%s: invalid or conflicting flag %q", path, name)
			}
			flags[name] = true
		}
		if flag.Forward && (flag.Value != "required" || flag.ValueCount > 1 || len(flag.Choices) != 0) {
			return fmt.Errorf("%s: forwarded commands require exactly one command name", path)
		}
		if flag.Terminator != "" && (!flag.Forward || strings.ContainsAny(flag.Terminator, " \t\r\n\x00")) {
			return fmt.Errorf("%s: invalid forwarding terminator", path)
		}
		choices := make(map[string]bool)
		for _, choice := range flag.Choices {
			if strings.TrimSpace(choice) == "" || strings.ContainsAny(choice, "\r\n\x00\x1b") || choices[choice] {
				return fmt.Errorf("%s: invalid or repeated choice for %s", path, flag.Name)
			}
			choices[choice] = true
		}
	}
	if c.StopFlagsAfterOperand && c.FlagsAfterPositionals {
		return fmt.Errorf("%s: conflicting positional flag rules", path)
	}
	for _, form := range c.Forms {
		if form.MinArgs < 0 || (form.MaxArgs != nil && *form.MaxArgs < form.MinArgs) {
			return fmt.Errorf("%s: invalid positional argument bounds", path)
		}
		seen := make(map[string]bool)
		for _, name := range append(slices.Clone(form.Required), form.Forbidden...) {
			flag := c.Flag(name)
			if flag == nil || seen[flag.Name] {
				return fmt.Errorf("%s: unknown or conflicting form flag %q", path, name)
			}
			seen[flag.Name] = true
		}
	}
	if c.Closed && len(c.Sources) == 0 {
		return fmt.Errorf("%s: a closed specification requires evidence", path)
	}
	names := make(map[string]bool)
	for i := range c.Commands {
		child := &c.Commands[i]
		if !validName(child.Name) || names[child.Name] {
			return fmt.Errorf("%s: invalid or conflicting command %q", path, child.Name)
		}
		names[child.Name] = true
		if err := validateCommand(child, path+" "+child.Name); err != nil {
			return err
		}
	}
	return nil
}

func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, " \t\r\n/\\=\x00") && !strings.HasPrefix(name, "-")
}

func validFlag(name string) bool {
	if strings.HasPrefix(name, "--") {
		return len(name) > 2 && !strings.HasPrefix(name[2:], "-") && !strings.ContainsAny(name[2:], " \t\r\n=/\\")
	}
	// Single-dash long flags (such as Go's -race) are also supported.
	return strings.HasPrefix(name, "-") && len(name) > 1 && !strings.ContainsAny(name[1:], " \t\r\n=/\\")
}

func validateEvidence(evidence []Evidence) error {
	for _, item := range evidence {
		if (item.Kind != "help" && item.Kind != "reference" && item.Kind != "manual") || strings.TrimSpace(item.Reference) == "" {
			return fmt.Errorf("specification evidence requires a help, manual or reference source")
		}
	}
	return nil
}

func (c *Command) Child(name string) *Command {
	for i := range c.Commands {
		if c.Commands[i].Name == name {
			return &c.Commands[i]
		}
	}
	if !c.AliasesComplete {
		return nil // Do not choose an alias before sibling conflicts can be checked.
	}
	var alias *Command
	for i := range c.Commands {
		if slices.Contains(c.Commands[i].Aliases, name) {
			if alias != nil {
				return nil // Conflicting aliases are not a usable command mapping.
			}
			alias = &c.Commands[i]
		}
	}
	return alias
}

func (c *Command) Flag(name string) *Flag {
	for i := range c.Flags {
		if c.Flags[i].Name == name || slices.Contains(c.Flags[i].Aliases, name) {
			return &c.Flags[i]
		}
	}
	return nil
}
