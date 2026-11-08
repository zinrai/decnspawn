package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var commands = map[string]func(Config) error{
	"create":  createAll,
	"start":   startAll,
	"stop":    stopAll,
	"destroy": destroyAll,
}

func run() error {
	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("missing subcommand")
	}
	if os.Args[1] == "version" {
		printVersion()
		return nil
	}
	command, ok := commands[os.Args[1]]
	if !ok {
		printUsage()
		return fmt.Errorf("%q is not a valid subcommand", os.Args[1])
	}

	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	file := flags.String("f", "", "Path to the YAML file with network and container definitions")
	flags.Parse(os.Args[2:])
	if *file == "" {
		flags.PrintDefaults()
		return fmt.Errorf("missing -f <yaml-file>")
	}

	config, err := ParseFile(*file)
	if err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("%s must be run as root", os.Args[1])
	}
	return command(config)
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  decnspawn create  -f <yaml-file>")
	fmt.Println("  decnspawn start   -f <yaml-file>")
	fmt.Println("  decnspawn stop    -f <yaml-file>")
	fmt.Println("  decnspawn destroy -f <yaml-file>")
	fmt.Println("  decnspawn version")
}
