// im-migrate upgrades the database explicitly, outside the server lifecycle.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"gochat/internal/pkg/config"
	"gochat/internal/pkg/migration"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "im-migrate:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("im-migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	filename := flags.String("config", "internal/config/config-dev.yaml", "server configuration (only mysql settings are required)")
	version := flags.Int("version", -1, "completed version for baseline or repair")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: im-migrate [-config file] status|up|baseline -version N|repair -version N")
		flags.PrintDefaults()
	}
	// Accept flags before or after the subcommand, using one documented syntax.
	command, options := "", []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-config" || a == "--config" || a == "-version" || a == "--version" {
			options = append(options, a)
			if i+1 < len(args) {
				i++
				options = append(options, args[i])
			}
		} else if len(a) > 0 && a[0] == '-' {
			options = append(options, a)
		} else if command == "" {
			command = a
		} else {
			return fmt.Errorf("unexpected argument %q", a)
		}
	}
	if err := flags.Parse(options); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch command {
	case "status", "up", "baseline", "repair":
	default:
		flags.Usage()
		return errors.New("choose status, up, baseline or repair")
	}
	if command == "baseline" || command == "repair" {
		if *version < 0 || *version > migration.LatestVersion {
			return fmt.Errorf("-version must be between 0 and %d", migration.LatestVersion)
		}
	} else if *version != -1 {
		return errors.New("-version is only supported by baseline and repair")
	}
	cfg, err := config.Load(*filename)
	if err != nil {
		return err
	}
	db, err := migration.Open(ctx, cfg.MySQL, command != "status")
	if err != nil {
		return err
	}
	defer db.Close()
	runner := migration.New(db)
	switch command {
	case "up":
		err = runner.Up(ctx)
	case "baseline":
		err = runner.Baseline(ctx, *version)
	case "repair":
		err = runner.Repair(ctx, *version)
	}
	if err != nil {
		return err
	}
	status, err := runner.Status(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "version: %d\ndirty: %t\nlatest: %d\npending: %v\n", status.Version, status.Dirty, migration.LatestVersion, status.Pending)
	if status.Version == -1 {
		if status.Empty {
			fmt.Fprintln(out, "empty database: run up to initialize")
		} else {
			fmt.Fprintf(out, "baseline candidate: %d (structure only; verify historical data repairs before baseline)\n", status.Candidate)
		}
	}
	if status.SchemaError != "" {
		fmt.Fprintln(out, "schema:", status.SchemaError)
	}
	return nil
}
