// Package cli holds the thin cobra command groups. Commands parse flags and
// delegate to internal packages; no business logic lives here.
package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/a2ngerer/claude-containers/internal/compose"
	"github.com/a2ngerer/claude-containers/internal/environment"
)

// envOpener resolves the bound environment for the current workspace. The CLI
// root injects the real opener; tests inject a stub.
type envOpener func() (*environment.Environment, error)

// newNewCmd builds the `new` command.
func newNewCmd(open envOpener) *cobra.Command {
	var template, from, extends string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new persona",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if template != "" && from != "" {
				return fmt.Errorf("--template and --from are mutually exclusive")
			}
			env, err := open()
			if err != nil {
				return err
			}

			var sc personaScaffold
			switch {
			case from != "":
				sc, err = copyPersonaScaffold(env, from)
				if err != nil {
					return err
				}
			case template != "":
				var ok bool
				sc, ok = personaTemplate(template)
				if !ok {
					return fmt.Errorf("unknown template %q (have: coder, reviewer)", template)
				}
			default:
				sc = blankScaffold(name, extends)
			}

			if err := scaffoldPersona(env, name, sc); err != nil {
				return err
			}
			// apply the chosen extends layer (also the default "_base") after scaffolding
			p, err := env.LoadPersona(name)
			if err != nil {
				return err
			}
			p.Extends = extends
			if err := env.SavePersona(p); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created persona %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "scaffold from an embedded template (coder|reviewer)")
	cmd.Flags().StringVar(&from, "from", "", "copy an existing persona as the starting point")
	cmd.Flags().StringVar(&extends, "extends", "_base", "layer this persona extends")
	return cmd
}

// blankScaffold returns a minimal persona.toml + CLAUDE.md for `new` without a
// template or source.
func blankScaffold(name, extends string) personaScaffold {
	if extends == "" {
		extends = "_base"
	}
	body := fmt.Sprintf(`name        = %q
description = ""
extends     = %q

[config]
claude_md       = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode    = "allowlist"
include = []

[config.subagents]
include = []

[config.mcp]
config = ""
strict = false

[enforcement]
permission_mode = "default"

[enforcement.tools]
allow = ["Read", "Grep", "Glob"]
deny  = []

[metadata]
version = "0.1.0"
author  = ""
`, name, extends)
	return personaScaffold{TOML: body, ClaudeMD: "# " + name + "\n"}
}

// newShowCmd builds the `show` command.
func newShowCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "show <persona>",
		Short: "Show a persona's composed manifest and capability preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			rm, err := compose.Compose(env, args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), formatShow(env, rm))
			return nil
		},
	}
}

// newEditCmd builds the `edit` command.
func newEditCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <persona>",
		Short: "Open a persona's persona.toml in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			if _, err := env.LoadPersona(args[0]); err != nil {
				return err
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			path := personaTOMLPath(env, args[0])
			ed := exec.Command(editor, path)
			ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
			return ed.Run()
		},
	}
}

// newRmCmd builds the `rm` command.
func newRmCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <persona>",
		Short: "Remove a persona",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			if err := removePersona(env, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed persona %q\n", args[0])
			return nil
		},
	}
}
