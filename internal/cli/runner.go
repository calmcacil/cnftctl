package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/calmcacil/cnftctl/internal/app"
)

type Options struct {
	Version string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Service app.Service
}

type Runner struct {
	root    *command
	version string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	service app.Service
}

func New(options Options) *Runner {
	version := options.Version
	if version == "" {
		version = "dev"
	}
	stdin := options.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := options.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	service := options.Service
	if service == nil {
		service = app.StubService{}
	}

	return &Runner{
		root:    commandTree(),
		version: version,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		service: service,
	}
}

func (r *Runner) Run(args []string) int {
	if err := r.run(context.Background(), args); err != nil {
		if errors.Is(err, errHelp) {
			return 0
		}
		if app.IsHealthError(err) {
			return 1
		}
		fmt.Fprintf(r.stderr, "cnftctl: %v\n", err)
		return 2
	}
	return 0
}

var errHelp = errors.New("help requested")

func (r *Runner) run(ctx context.Context, args []string) error {
	match, err := r.parse(args)
	if err != nil {
		return err
	}
	if match.version {
		fmt.Fprintf(r.stdout, "cnftctl %s\n", r.version)
		return nil
	}
	if match.help {
		r.writeHelp(match.path, match.command)
		return errHelp
	}
	if match.command.HandlerName == "" {
		r.writeHelp(match.path, match.command)
		return errHelp
	}

	request := app.CommandRequest{
		Command: match.command.HandlerName,
		Args:    match.positionals,
		Flags:   match.flags,
		Environment: map[string]string{
			"SSH_CONNECTION": os.Getenv("SSH_CONNECTION"),
			"SSH_CLIENT":     os.Getenv("SSH_CLIENT"),
		},
	}
	return r.service.Run(ctx, app.IO{Stdin: r.stdin, Stdout: r.stdout, Stderr: r.stderr}, request)
}

type parseResult struct {
	command     *command
	path        []string
	flags       map[string][]string
	positionals []string
	help        bool
	version     bool
}

func (r *Runner) parse(args []string) (parseResult, error) {
	result := parseResult{command: r.root, path: []string{"cnftctl"}, flags: map[string][]string{}}
	knownFlags := collectFlags(r.root, nil)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			result.positionals = append(result.positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			name, value, consumed, err := parseFlag(args, i, knownFlags)
			if err != nil {
				return result, err
			}
			if name == "help" {
				result.help = value == "true"
			} else if name == "version" {
				result.version = value == "true"
			}
			spec := knownFlags[name]
			if !spec.Repeat && len(result.flags[name]) > 0 {
				return result, fmt.Errorf("flag --%s may not be repeated", name)
			}
			result.flags[name] = append(result.flags[name], value)
			i += consumed
			continue
		}

		child := result.command.findChild(arg)
		if child == nil {
			result.positionals = append(result.positionals, arg)
			continue
		}
		result.command = child
		result.path = append(result.path, child.Use)
		knownFlags = collectFlags(r.root, child)
	}

	for _, spec := range knownFlags {
		if spec.Default != "" && len(result.flags[spec.Name]) == 0 {
			result.flags[spec.Name] = []string{spec.Default}
		}
	}

	if result.version {
		if len(result.positionals) != 0 || len(result.path) != 1 {
			return result, errors.New("--version does not accept a command or positional arguments")
		}
		return result, nil
	}
	if !result.help && len(result.command.Children) == 0 {
		min, max := positionalArity(result.command.Args)
		if len(result.positionals) < min || len(result.positionals) > max {
			return result, fmt.Errorf("%s expects %d..%d positional argument(s), got %d", join(result.path, " "), min, max, len(result.positionals))
		}
	}
	groups := map[string]string{}
	for _, spec := range knownFlags {
		if spec.Name == "" || spec.Exclusive == "" || len(result.flags[spec.Name]) == 0 {
			continue
		}
		if other, ok := groups[spec.Exclusive]; ok && other != spec.Name {
			return result, fmt.Errorf("flags --%s and --%s are mutually exclusive", other, spec.Name)
		}
		groups[spec.Exclusive] = spec.Name
	}
	if !result.help && len(result.command.Children) > 0 && len(result.positionals) > 0 {
		return result, fmt.Errorf("unknown command %q for %s", result.positionals[0], join(result.path, " "))
	}
	if !result.help {
		reporting := map[string]bool{"status": true, "doctor": true, "validate": true, "plan": true, "transactions list": true, "ddns status": true}
		if !reporting[result.command.HandlerName] && (len(result.flags["output"]) > 0 || result.flags["detail"] != nil) {
			return result, fmt.Errorf("--output and --detail are only supported by reporting commands")
		}
		if result.flags["detail"] != nil && result.flags["detail"][0] == "false" {
			delete(result.flags, "detail")
		}
	}
	return result, nil
}

func positionalArity(args string) (int, int) {
	if strings.TrimSpace(args) == "" {
		return 0, 0
	}
	min := 0
	max := 0
	for _, field := range strings.Fields(args) {
		max++
		if !strings.HasPrefix(field, "[") {
			min++
		}
	}
	return min, max
}

func collectFlags(root, current *command) map[string]flagSpec {
	flags := map[string]flagSpec{}
	for _, spec := range root.Flags {
		flags[spec.Name] = spec
		if spec.Short != "" {
			flags[spec.Short] = spec
		}
	}
	if current != nil {
		for _, spec := range current.Flags {
			flags[spec.Name] = spec
			if spec.Short != "" {
				flags[spec.Short] = spec
			}
		}
	}
	return flags
}

func parseFlag(args []string, index int, known map[string]flagSpec) (string, string, int, error) {
	arg := args[index]
	name := ""
	value := ""

	if strings.HasPrefix(arg, "--") {
		nameValue := strings.TrimPrefix(arg, "--")
		name, value, _ = strings.Cut(nameValue, "=")
	} else {
		name = strings.TrimPrefix(arg, "-")
	}

	spec, ok := known[name]
	if !ok {
		return "", "", 0, fmt.Errorf("unknown flag %s", arg)
	}
	name = spec.Name
	if spec.Bool {
		if value == "" {
			value = "true"
		}
		if value != "true" && value != "false" {
			return "", "", 0, fmt.Errorf("flag --%s expects true or false", name)
		}
		return name, value, 0, nil
	}

	if value != "" {
		return name, value, 0, nil
	}
	if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
		return "", "", 0, fmt.Errorf("flag --%s requires a value", name)
	}
	return name, args[index+1], 1, nil
}

func (r *Runner) writeHelp(path []string, c *command) {
	fmt.Fprintf(r.stdout, "%s\n\n", c.Summary)
	fmt.Fprintf(r.stdout, "Usage:\n  %s\n", usageFor(path, c))
	if len(c.Children) > 0 {
		fmt.Fprintln(r.stdout, "\nCommands:")
		for _, child := range c.Children {
			fmt.Fprintf(r.stdout, "  %-18s %s\n", child.Use, child.Summary)
		}
	}

	flags := c.Flags
	if c != r.root {
		flags = append(append([]flagSpec(nil), r.root.Flags...), c.Flags...)
	}
	if len(flags) > 0 {
		fmt.Fprintln(r.stdout, "\nFlags:")
		sort.SliceStable(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
		for _, flag := range flags {
			name := "--" + flag.Name
			if flag.Short != "" {
				name = fmt.Sprintf("-%s, %s", flag.Short, name)
			}
			if !flag.Bool {
				name += " " + flag.ValueName
			}
			usage := flag.Usage
			if flag.Default != "" {
				usage += fmt.Sprintf(" (default %s)", flag.Default)
			}
			fmt.Fprintf(r.stdout, "  %-28s %s\n", name, usage)
		}
	}
}

func join(values []string, sep string) string {
	return strings.Join(values, sep)
}
