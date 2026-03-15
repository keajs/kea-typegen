package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"kea-typegen/rewrite/internal/keainspect"
	"kea-typegen/rewrite/internal/tsgoapi"
)

const invocationDirEnv = "KEA_TYPEGEN_CWD"

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(os.Args) == 1 {
		printTypegenUsage(os.Stdout)
		fmt.Fprintln(os.Stderr, "Not enough non-option arguments: got 0, need at least 1")
		os.Exit(1)
	}

	if isHelpArgument(os.Args[1]) {
		printTypegenUsage(os.Stdout)
		return
	}

	if len(os.Args) > 1 && isAuxCommand(os.Args[1]) {
		if err := runAuxCommand(ctx, os.Args[1], os.Args[2:]); err != nil {
			fail(err)
		}
		return
	}

	if len(os.Args) > 1 && isTypegenCommand(os.Args[1]) {
		if err := runTypegenCommand(ctx, os.Args[1], os.Args[2:]); err != nil {
			fail(err)
		}
		return
	}

	if err := runLegacyCommand(); err != nil {
		fail(err)
	}
}

func isHelpArgument(argument string) bool {
	switch argument {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func isTypegenCommand(argument string) bool {
	switch argument {
	case "check", "write", "watch":
		return true
	default:
		return false
	}
}

func isAuxCommand(argument string) bool {
	switch argument {
	case "probe-api":
		return true
	default:
		return false
	}
}

func printTypegenUsage(output io.Writer) {
	lines := []string{
		"kea-typegen-go <command>",
		"",
		"Commands:",
		"  kea-typegen-go check  - check what should be done",
		"  kea-typegen-go write  - write logicType.ts files",
		"  kea-typegen-go watch  - watch for changes and write logicType.ts files",
		"  kea-typegen-go probe-api - probe hidden tsgo API methods",
		"",
		"Options:",
		"  -c, --config               Path to tsconfig.json (otherwise auto-detected)",
		"  -f, --file                 Single file to evaluate (can't be used with --config)",
		"  -r, --root                 Root for logic paths. E.g: ./frontend/src",
		"  -t, --types                Folder to write logicType.ts files to.",
		"  -q, --quiet                Write nothing to stdout",
		"      --no-import            Do not automatically import generated types in logic files",
		"      --write-paths          Write paths into logic files that have none",
		"      --delete               Delete logicType.ts files without a corresponding logic.ts",
		"      --add-ts-nocheck       Add @ts-nocheck to top of logicType.ts files",
		"      --convert-to-builders  Convert Kea 2.0 inputs to Kea 3.0 logic builders",
		"      --import-global-types  Add import statements in logicType.ts files for global types",
		"      --ignore-import-paths  List of paths we will never import from inside logicType.ts files",
		"      --show-ts-errors       Show TypeScript errors",
		"      --use-cache            Cache generated logic files into .typegen",
		"      --verbose              Slightly more verbose output log",
		"      --tsgo-bin             Path to the tsgo binary",
		"      --timeout              Per-request timeout",
		"      --poll-interval        Watch polling interval",
		"      --help                 Show help",
	}
	for _, line := range lines {
		fmt.Fprintln(output, line)
	}
}

func runAuxCommand(ctx context.Context, command string, args []string) error {
	switch command {
	case "probe-api":
		return runProbeAPICommand(ctx, args)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func runTypegenCommand(ctx context.Context, command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	baseDir := findBaseDir()
	var options keainspect.AppOptions
	var ignoreImportPaths stringSliceFlag

	options.Timeout = 15 * time.Second
	options.PollInterval = time.Second

	flags.StringVar(&options.TsConfigPath, "config", "", "Path to tsconfig.json (otherwise auto-detected)")
	flags.StringVar(&options.TsConfigPath, "c", "", "Path to tsconfig.json (otherwise auto-detected)")
	flags.StringVar(&options.SourceFilePath, "file", "", "Single file to evaluate (can't be used with --config)")
	flags.StringVar(&options.SourceFilePath, "f", "", "Single file to evaluate (can't be used with --config)")
	flags.StringVar(&options.RootPath, "root", "", "Root for logic paths. E.g: ./frontend/src")
	flags.StringVar(&options.RootPath, "r", "", "Root for logic paths. E.g: ./frontend/src")
	flags.StringVar(&options.TypesPath, "types", "", "Folder to write logicType.ts files to.")
	flags.StringVar(&options.TypesPath, "t", "", "Folder to write logicType.ts files to.")
	flags.BoolVar(&options.Quiet, "quiet", false, "Write nothing to stdout")
	flags.BoolVar(&options.Quiet, "q", false, "Write nothing to stdout")
	flags.BoolVar(&options.NoImport, "no-import", false, "Do not automatically import generated types in logic files")
	flags.BoolVar(&options.WritePaths, "write-paths", false, "Write paths into logic files that have none")
	flags.BoolVar(&options.Delete, "delete", false, "Delete logicType.ts files without a corresponding logic.ts")
	flags.BoolVar(&options.AddTsNocheck, "add-ts-nocheck", false, "Add @ts-nocheck to top of logicType.ts files")
	flags.BoolVar(&options.ConvertToBuilders, "convert-to-builders", false, "Convert Kea 2.0 inputs to Kea 3.0 logic builders")
	flags.BoolVar(&options.ImportGlobalTypes, "import-global-types", false, "Add import statements in logicType.ts files for global types")
	flags.Var(&ignoreImportPaths, "ignore-import-paths", "List of paths we will never import from inside logicType.ts files")
	flags.BoolVar(&options.ShowTSErrors, "show-ts-errors", false, "Show TypeScript errors")
	flags.BoolVar(&options.UseCache, "use-cache", false, "Cache generated logic files into .typegen")
	flags.BoolVar(&options.Verbose, "verbose", false, "Slightly more verbose output log")
	flags.StringVar(&options.BinaryPath, "tsgo-bin", tsgoapi.PreferredBinary(baseDir), "Path to the tsgo binary")
	flags.DurationVar(&options.Timeout, "timeout", 15*time.Second, "Per-request timeout")
	flags.DurationVar(&options.PollInterval, "poll-interval", time.Second, "Watch polling interval")

	if err := flags.Parse(args); err != nil {
		return err
	}

	options.IgnoreImportPaths = ignoreImportPaths
	options.Write = command != "check"
	options.Watch = command == "watch"

	resolved, err := keainspect.ResolveAppOptions(options, visitedFlags(flags), cliWorkingDir())
	if err != nil {
		return err
	}
	return keainspect.RunTypegen(ctx, resolved)
}

func runLegacyCommand() error {
	baseDir := findBaseDir()

	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	projectDir := flags.String("project-dir", filepath.Join(baseDir, "samples"), "TypeScript project directory")
	configFile := flags.String("config", filepath.Join(baseDir, "samples", "tsconfig.json"), "tsconfig path")
	filePath := flags.String("file", filepath.Join(baseDir, "samples", "logic.ts"), "TypeScript file with kea logic")
	tsgoBin := flags.String("tsgo-bin", tsgoapi.PreferredBinary(baseDir), "Path to the tsgo binary")
	format := flags.String("format", "report", "Output format: report, model, or typegen")
	timeout := flags.Duration("timeout", 15*time.Second, "Per-request timeout")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}

	report, err := keainspect.InspectFile(context.Background(), keainspect.InspectOptions{
		BinaryPath: mustAbsFrom(cliWorkingDir(), *tsgoBin),
		ProjectDir: mustAbsFrom(cliWorkingDir(), *projectDir),
		ConfigFile: mustAbsFrom(cliWorkingDir(), *configFile),
		File:       mustAbsFrom(cliWorkingDir(), *filePath),
		Timeout:    *timeout,
	})
	if err != nil {
		return err
	}

	switch *format {
	case "report":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "model":
		model, err := keainspect.BuildParsedLogics(report)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(model)
	case "typegen":
		model, err := keainspect.BuildParsedLogics(report)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(os.Stdout, keainspect.EmitTypegen(model))
		return err
	default:
		return fmt.Errorf("unknown format %q", *format)
	}
}

func visitedFlags(flags *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	flags.Visit(func(item *flag.Flag) {
		visited[item.Name] = true
	})
	return visited
}

func findBaseDir() string {
	wd := mustGetwd()
	if _, err := os.Stat(filepath.Join(wd, "rewrite", "go.mod")); err == nil {
		return wd
	}
	return mustAbsFrom(wd, "..")
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	return wd
}

func cliWorkingDir() string {
	if value := strings.TrimSpace(os.Getenv(invocationDirEnv)); value != "" {
		return mustAbsFrom(mustGetwd(), value)
	}
	return mustGetwd()
}

func mustAbsFrom(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	abs, err := filepath.Abs(filepath.Join(base, path))
	if err != nil {
		fail(err)
	}
	return abs
}

func fail(err error) {
	var exitErr *keainspect.ExitCodeError
	if errorsAs(err, &exitErr) {
		if exitErr.Err != nil {
			fmt.Fprintln(os.Stderr, exitErr.Err)
		}
		os.Exit(exitErr.Code)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func errorsAs(err error, target any) bool {
	type unwrapper interface {
		Unwrap() error
	}
	current := err
	for current != nil {
		switch typed := target.(type) {
		case **keainspect.ExitCodeError:
			if value, ok := current.(*keainspect.ExitCodeError); ok {
				*typed = value
				return true
			}
		}
		unwrapped, ok := current.(unwrapper)
		if !ok {
			return false
		}
		current = unwrapped.Unwrap()
	}
	return false
}
