package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"git.unix.lgbt/diamondburned/cronmon/cronmon"
	"git.unix.lgbt/diamondburned/cronmon/cronmon/journal"
)

var (
	journalFile string
	scriptsDir  string
)

func init() {
	configDir, err := os.UserConfigDir()
	if err == nil {
		scriptsDir = filepath.Join(configDir, "cronmon", "scripts")
		journalFile = filepath.Join(configDir, "cronmon", "journal.json")
	}

	flag.StringVar(&journalFile, "j", journalFile, "journal file path")
	flag.StringVar(&scriptsDir, "s", scriptsDir, "scripts directory path")
	flag.Usage = func() {
		f := func(f string, v ...interface{}) {
			fmt.Fprintf(flag.CommandLine.Output(), f, v...)
		}

		f("Usage:\n")
		f("  %s -j <journal> -s <scripts> [|cron]\n", filepath.Base(os.Args[0]))
		f("\n")
		f("Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if journalFile == "" {
		log.Fatalln("missing -j path to journal file")
	}
	if scriptsDir == "" {
		log.Fatalln("missing -s path to script directory")
	}

	// Ensure that, if the scripts directory exists, that it is an actual
	// directory.
	if stat, err := os.Stat(scriptsDir); err == nil && !stat.IsDir() {
		log.Fatalln("scripts path", scriptsDir, "is not directory")
	}
}

func main() {
	switch flag.Arg(0) {
	case "cron":
		cron()
	case "":
		start()
	default:
		log.Fatalf("unknown subcommand %q\n", flag.Arg(0))
	}
}

func cron() {
	crontimes := [...]string{
		"# Start cronmon immediately on startup.",
		"@reboot",
		"# Monitor cronmon's status every minute.",
		"* * * * *",
	}

	j := strconv.Quote(journalFile)
	s := strconv.Quote(scriptsDir + "/")

	for _, crontime := range crontimes {
		if strings.HasPrefix(crontime, "#") {
			fmt.Println(crontime)
			continue
		}

		fmt.Println(crontime, os.Args[0], "-j", j, "-s", s)
	}
}

func start() {
	j, err := journal.NewFileLockJournaler(journalFile)
	if err != nil {
		if errors.Is(err, journal.ErrLockedElsewhere) {
			// Non-fatal error.
			log.Println("cronmon is already running")
			return
		}

		log.Fatalln("failed to acquire journal lock:", err)
	}
	defer j.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Beware: changing the combination of these writers will break existing
	// status directories.
	journaler := journal.MultiWriter(j, journal.NewHumanWriter("stdout", os.Stdout))

	m, err := cronmon.NewMonitor(ctx, scriptsDir, journaler)
	if err != nil {
		log.Fatalln("failed to restore monitor:", err)
	}
	defer m.Stop()

	<-ctx.Done()
}
