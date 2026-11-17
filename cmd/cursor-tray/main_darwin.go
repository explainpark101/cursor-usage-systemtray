//go:build darwin

// cursor-tray (macOS): polls Cursor usage and writes a RunCat Neo Custom
// Metrics JSON file. Point RunCat Neo at that file under
// Settings → Metrics → Custom Metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cursor-tray/internal/auth"
	"cursor-tray/internal/cursorapi"
	"cursor-tray/internal/poller"
	"cursor-tray/internal/runcat"
	"cursor-tray/internal/storage"
	"cursor-tray/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "set-cookie":
			os.Exit(runSetCookie(os.Args[2:]))
		case "clear-cookie":
			os.Exit(runClearCookie())
		case "once":
			os.Exit(runOnce(os.Args[2:]))
		case "help", "-h", "--help":
			printUsage(os.Stdout)
			return
		case "version", "-version", "--version":
			fmt.Println(version.Version)
			return
		}
	}

	os.Exit(runDaemon(os.Args[1:]))
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `cursor-tray %s — Cursor usage → RunCat Neo Custom Metrics (macOS)

Usage:
  cursor-tray [flags]           Poll forever and update the metrics JSON
  cursor-tray once [flags]      Fetch once and exit
  cursor-tray set-cookie [-f file | cookie...]
                                Save a Cookie header or Netscape cookies.txt
  cursor-tray clear-cookie      Remove the stored cookie
  cursor-tray version

Flags (daemon / once):
  -out PATH     Metrics JSON path (default: $RUNCAT_OUT_FILE or
                ~/Library/Application Support/cursor-tray/runcat-usage.json)
  -interval N   Poll interval seconds (default: config or 10)

Environment:
  RUNCAT_OUT_FILE   Override metrics output path
  CURSOR_COOKIE     Cookie header used when no stored cookie exists

RunCat Neo setup:
  1. cursor-tray set-cookie -f cookies.txt   # or paste Cookie header
  2. cursor-tray                            # or install the launchd plist
  3. RunCat Neo → Settings → Metrics → Custom Metrics → Add
     → pick the runcat-usage.json file

Schema: https://github.com/runcat-dev/RunCatNeo/blob/main/docs/CustomMetricsSchema.md
`, version.Version)
}

func openStore() (*storage.Store, error) {
	return storage.Open()
}

func resolveCookie(store *storage.Store) string {
	if c := store.Cookie(); c != "" {
		return c
	}
	return strings.TrimSpace(os.Getenv("CURSOR_COOKIE"))
}

func resolveOut(store *storage.Store, flagOut string) string {
	if flagOut != "" {
		return flagOut
	}
	return runcat.DefaultOutputPath(store.Dir)
}

func writeEvent(out string, ev poller.Event) error {
	var snap runcat.Snapshot
	switch {
	case ev.Unauthorized:
		snap = runcat.StatusSnapshot("login required", "")
	case ev.Err != nil:
		snap = runcat.StatusSnapshot(truncateMsg(ev.Err.Error(), 80), "")
	case ev.Usage != nil:
		snap = runcat.FromUsage(ev.Usage)
	default:
		snap = runcat.StatusSnapshot("no data", "")
	}
	return runcat.WriteAtomic(out, snap)
}

func runDaemon(args []string) int {
	fs := flag.NewFlagSet("cursor-tray", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outFlag := fs.String("out", "", "metrics JSON output path")
	interval := fs.Int("interval", 0, "poll interval seconds")
	if err := fs.Parse(args); err != nil {
		printUsage(os.Stderr)
		return 2
	}

	store, err := openStore()
	if err != nil {
		log.Printf("storage: %v", err)
		return 1
	}
	out := resolveOut(store, *outFlag)

	cfgInterval := store.Config().PollIntervalSeconds
	if *interval > 0 {
		cfgInterval = *interval
	}
	if cfgInterval <= 0 {
		cfgInterval = 10
	}

	logPath := filepath.Join(store.Dir, "cursor-tray.log")
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
		defer f.Close()
	}

	log.Printf("cursor-tray %s (runcat-neo) starting; out=%s interval=%ds", version.Version, out, cfgInterval)

	events := make(chan poller.Event, 4)
	p := &poller.Poller{
		Out: events,
		Cookie: func() string {
			return resolveCookie(store)
		},
		MinInterval: time.Duration(cfgInterval) * time.Second,
		MaxInterval: 5 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go p.Run(ctx)
	p.RefreshNow()

	for {
		select {
		case <-ctx.Done():
			p.Stop()
			log.Printf("stopped")
			return 0
		case ev := <-events:
			if err := writeEvent(out, ev); err != nil {
				log.Printf("write metrics: %v", err)
				continue
			}
			switch {
			case ev.Unauthorized:
				log.Printf("unauthorized — run: cursor-tray set-cookie")
			case ev.Err != nil:
				log.Printf("poll error: %v", ev.Err)
			default:
				log.Printf("updated %s (total %.1f%%)", out, ev.Usage.TotalPercentUsed)
			}
		}
	}
}

func runOnce(args []string) int {
	fs := flag.NewFlagSet("once", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outFlag := fs.String("out", "", "metrics JSON output path")
	if err := fs.Parse(args); err != nil {
		printUsage(os.Stderr)
		return 2
	}

	store, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "storage: %v\n", err)
		return 1
	}
	out := resolveOut(store, *outFlag)
	cookie := resolveCookie(store)

	ev := poller.Event{}
	if cookie == "" {
		ev.Unauthorized = true
		ev.Err = cursorapi.ErrUnauthorized
	} else {
		usage, err := cursorapi.Fetch(context.Background(), cookie)
		ev.Usage = usage
		ev.Err = err
		ev.Unauthorized = errors.Is(err, cursorapi.ErrUnauthorized)
	}

	if err := writeEvent(out, ev); err != nil {
		fmt.Fprintf(os.Stderr, "write metrics: %v\n", err)
		return 1
	}
	fmt.Println(out)
	if ev.Err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", ev.Err)
		return 1
	}
	return 0
}

func runSetCookie(args []string) int {
	fs := flag.NewFlagSet("set-cookie", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("f", "", "path to Cookie header file or Netscape cookies.txt")
	if err := fs.Parse(args); err != nil {
		printUsage(os.Stderr)
		return 2
	}

	var raw string
	switch {
	case *file != "":
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read cookie file: %v\n", err)
			return 1
		}
		raw = string(b)
	case fs.NArg() > 0:
		raw = strings.Join(fs.Args(), " ")
	default:
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
			return 1
		}
		raw = string(b)
	}

	header, err := auth.ParseCookieInput(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse cookie: %v\n", err)
		return 1
	}
	if header == "" {
		fmt.Fprintln(os.Stderr, "empty cookie")
		return 1
	}

	store, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "storage: %v\n", err)
		return 1
	}
	if err := store.SetCookie(header); err != nil {
		fmt.Fprintf(os.Stderr, "save cookie: %v\n", err)
		return 1
	}
	fmt.Println("cookie saved")
	return 0
}

func runClearCookie() int {
	store, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "storage: %v\n", err)
		return 1
	}
	if err := store.SetCookie(""); err != nil {
		fmt.Fprintf(os.Stderr, "clear cookie: %v\n", err)
		return 1
	}
	fmt.Println("cookie cleared")
	return 0
}

func truncateMsg(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
