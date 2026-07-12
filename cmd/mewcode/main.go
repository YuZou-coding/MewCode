package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"mewcode/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(runCLI(ctx, os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func runCLI(ctx context.Context, args []string, input io.Reader, output io.Writer, errorsOut io.Writer) int {
	if len(args) > 1 && args[1] == "setup-global" {
		if err := runSetupGlobal(args[2:], output); err != nil {
			fmt.Fprintln(errorsOut, err)
			return 1
		}
		return 0
	}

	resume := false
	nextArgs := args[:1]
	for _, arg := range args[1:] {
		if arg == "--resume" {
			resume = true
			continue
		}
		nextArgs = append(nextArgs, arg)
	}
	os.Args = nextArgs
	err := app.RunWithResume(ctx, input, output, errorsOut, resume)
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(errorsOut, err)
		return 1
	}
	return 0
}

func setupGlobalHelp() string {
	return `mewcode setup-global

Usage:
  mewcode setup-global [--from <project>] [--include a,b] [--all] [--dry-run] [--force]

默认复制: mewcode.yaml, MEWCODE.md
可选复制: permissions, hooks, notes, skills, workers, servers
永不复制: sessions, artifacts, worktrees, teams

Examples:
  mewcode setup-global --from /Users/theone/Documents/Mewcode
  mewcode setup-global --from /Users/theone/Documents/Mewcode --include permissions,hooks
  mewcode setup-global --from /Users/theone/Documents/Mewcode --all --dry-run
`
}

type setupOptions struct {
	from    string
	include map[string]bool
	all     bool
	dryRun  bool
	force   bool
}

type setupResource struct {
	name     string
	source   string
	target   string
	required bool
	dir      bool
}

func runSetupGlobal(args []string, output io.Writer) error {
	options, help, err := parseSetupGlobalArgs(args)
	if err != nil {
		return err
	}
	if help {
		_, err := io.WriteString(output, setupGlobalHelp())
		return err
	}
	if options.from == "" {
		options.from, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	targetRoot := filepath.Join(home, ".mewcode")
	resources := setupResources(options, targetRoot)
	if err := os.MkdirAll(targetRoot, 0700); err != nil && !options.dryRun {
		return err
	}
	for _, resource := range resources {
		source := filepath.Join(options.from, resource.source)
		target := filepath.Join(targetRoot, resource.target)
		info, err := os.Stat(source)
		if err != nil {
			if os.IsNotExist(err) && !resource.required {
				fmt.Fprintf(output, "skip missing optional %s: %s\n", resource.name, source)
				continue
			}
			return fmt.Errorf("setup-global missing %s: %s", resource.name, source)
		}
		if resource.dir != info.IsDir() {
			return fmt.Errorf("setup-global %s type mismatch: %s", resource.name, source)
		}
		if options.dryRun {
			fmt.Fprintf(output, "would copy %s -> %s\n", source, target)
			continue
		}
		if err := copyPath(source, target, resource.dir, options.force); err != nil {
			return err
		}
		fmt.Fprintf(output, "copied %s -> %s\n", source, target)
	}
	return nil
}

func parseSetupGlobalArgs(args []string) (setupOptions, bool, error) {
	options := setupOptions{include: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--help", "-h":
			return options, true, nil
		case "--from":
			i++
			if i >= len(args) {
				return options, false, fmt.Errorf("--from requires a path")
			}
			options.from = args[i]
		case "--include":
			i++
			if i >= len(args) {
				return options, false, fmt.Errorf("--include requires a comma-separated list")
			}
			for _, name := range strings.Split(args[i], ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					options.include[name] = true
				}
			}
		case "--all":
			options.all = true
		case "--dry-run":
			options.dryRun = true
		case "--force":
			options.force = true
		default:
			return options, false, fmt.Errorf("unknown setup-global argument: %s", arg)
		}
	}
	return options, false, nil
}

func setupResources(options setupOptions, targetRoot string) []setupResource {
	_ = targetRoot
	resources := []setupResource{
		{name: "mewcode.yaml", source: "mewcode.yaml", target: "mewcode.yaml", required: true},
		{name: "MEWCODE.md", source: "MEWCODE.md", target: "MEWCODE.md"},
	}
	optionals := []setupResource{
		{name: "permissions", source: ".mewcode/permissions.yaml", target: "permissions.yaml"},
		{name: "hooks", source: ".mewcode/hooks.yaml", target: "hooks.yaml"},
		{name: "notes", source: ".mewcode/notes.md", target: "notes.md"},
		{name: "skills", source: ".mewcode/skills", target: "skills", dir: true},
		{name: "workers", source: ".mewcode/workers", target: "workers", dir: true},
		{name: "servers", source: ".mewcode/mcp_servers.yaml", target: "mcp_servers.yaml"},
	}
	for _, resource := range optionals {
		if options.all || options.include[resource.name] {
			resources = append(resources, resource)
		}
	}
	return resources
}

func copyPath(source string, target string, isDir bool, force bool) error {
	if !force {
		if _, err := os.Stat(target); err == nil {
			if err := os.Rename(target, target+".bak"); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if isDir {
		return copyDir(source, target)
	}
	return copyFile(source, target)
}

func copyFile(source string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyDir(source string, target string) error {
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		nextTarget := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(nextTarget, 0700)
		}
		return copyFile(path, nextTarget)
	})
}
