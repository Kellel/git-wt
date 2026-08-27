package wt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type App struct {
	cwd    string
	env    []string
	out    io.Writer
	err    io.Writer
	git    gitRunner
	rename func(string, string) error
}

type configOverrides struct {
	root           string
	namespace      string
	branchTemplate string
}

func New(cwd string, env []string, stdout, stderr io.Writer) *App {
	return &App{
		cwd:    cwd,
		env:    append([]string(nil), env...),
		out:    stdout,
		err:    stderr,
		git:    gitRunner{env: cleanGitEnvironment(env)},
		rename: os.Rename,
	}
}

func (a *App) Run(args []string) error {
	args, overrides, err := extractGlobalFlags(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		if err := a.printUsage(a.err); err != nil {
			return fmt.Errorf("write usage: %w", err)
		}
		return errors.New("a command is required")
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return a.printUsage(a.out)
	}
	if args[0] == "shell-init" {
		return a.runShellInit(args[1:])
	}

	cfg, err := a.loadConfig(overrides)
	if err != nil {
		return err
	}

	switch args[0] {
	case "clone":
		return a.runClone(cfg, args[1:])
	case "adopt":
		return a.runAdopt(cfg, args[1:])
	case "new":
		return a.runNew(cfg, args[1:])
	case "list":
		return a.runList(cfg, args[1:])
	case "path":
		return a.runPath(cfg, args[1:])
	case "pull":
		return a.runPull(cfg, args[1:])
	case "remove":
		return a.runRemove(cfg, args[1:])
	default:
		return fmt.Errorf("unknown command %q; run 'git wt --help' for usage", args[0])
	}
}

func (a *App) printUsage(w io.Writer) error {
	_, err := io.WriteString(w, `usage: git wt [global options] <command> [options]

Worktree-first project management.

Commands:
  clone       clone a repository into a managed project
  adopt       move a standalone checkout into a managed project
  new         create a task worktree and branch
  list        list a project's worktrees
  path        print a trunk, task, or notes path
  pull        fast-forward the project's trunk
  remove      safely remove a task worktree
  shell-init  print shell integration

Global options:
  --root <path>              workspace root (default: $HOME/code)
  --namespace <name>         default namespace
  --branch-template <value>  branch template containing %s (default: %s)
  -h, --help                 show this help

Configuration precedence is flags, environment (WT_ROOT, WT_NAMESPACE,
WT_BRANCH_TEMPLATE), git config (wt.root, wt.namespace, wt.branchTemplate),
then built-in defaults.
`)
	return err
}

func extractGlobalFlags(args []string) ([]string, configOverrides, error) {
	var result []string
	var overrides configOverrides
	afterTerminator := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			afterTerminator = true
			result = append(result, arg)
			continue
		}
		if afterTerminator {
			result = append(result, arg)
			continue
		}
		name, inlineValue, hasInline := strings.Cut(arg, "=")
		var target *string
		switch name {
		case "--root":
			target = &overrides.root
		case "--namespace":
			target = &overrides.namespace
		case "--branch-template":
			target = &overrides.branchTemplate
		default:
			result = append(result, arg)
			continue
		}

		if hasInline {
			if inlineValue == "" {
				return nil, configOverrides{}, fmt.Errorf("%s requires a value", name)
			}
			*target = inlineValue
			continue
		}
		if i+1 >= len(args) {
			return nil, configOverrides{}, fmt.Errorf("%s requires a value", name)
		}
		i++
		*target = args[i]
	}

	return result, overrides, nil
}

func parseOptions(args []string, valueOptions map[string]*string, boolOptions map[string]*bool) ([]string, bool, error) {
	var positional []string
	var help bool
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "--help" {
			help = true
			continue
		}
		name, inlineValue, hasInline := strings.Cut(arg, "=")
		if target, ok := boolOptions[name]; ok {
			if hasInline {
				return nil, false, fmt.Errorf("%s does not accept a value", name)
			}
			*target = true
			continue
		}
		if target, ok := valueOptions[name]; ok {
			if hasInline {
				if inlineValue == "" {
					return nil, false, fmt.Errorf("%s requires a value", name)
				}
				*target = inlineValue
				continue
			}
			if i+1 >= len(args) {
				return nil, false, fmt.Errorf("%s requires a value", name)
			}
			i++
			*target = args[i]
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return nil, false, fmt.Errorf("unknown option %q", arg)
		}
		positional = append(positional, arg)
	}
	return positional, help, nil
}

func absolutePath(base, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}
