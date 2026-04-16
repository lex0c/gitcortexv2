package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gitcortex/internal/extract"
	"gitcortex/internal/git"
	"gitcortex/internal/stats"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gitcortex",
		Short: "Git metrics extraction and analysis",
	}

	rootCmd.AddCommand(extractCmd())
	rootCmd.AddCommand(statsCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func extractCmd() *cobra.Command {
	var cfg extract.Config

	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract commit data from a Git repository into JSONL",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := filepath.Abs(cfg.Repo)
			if err != nil {
				return fmt.Errorf("resolve repo path: %w", err)
			}
			cfg.Repo = repoPath

			if !cmd.Flags().Changed("branch") {
				cfg.Branch = git.DetectDefaultBranch(repoPath)
			}

			if cfg.CommandTimeout == 0 {
				cfg.CommandTimeout = extract.DefaultCommandTimeout
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return extract.Run(ctx, cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Repo, "repo", ".", "Path to the Git repository")
	cmd.Flags().IntVar(&cfg.BatchSize, "batch-size", 1000, "Number of commits to process per batch")
	cmd.Flags().StringVar(&cfg.Output, "output", "git_data.jsonl", "Output JSONL file path")
	cmd.Flags().StringVar(&cfg.StateFile, "state-file", "git_state", "File to persist worker state")
	cmd.Flags().IntVar(&cfg.StartOffset, "start-offset", -1, "Number of commits to skip before processing")
	cmd.Flags().StringVar(&cfg.StartSHA, "start-sha", "", "Last processed commit SHA to resume after")
	cmd.Flags().StringVar(&cfg.Branch, "branch", "main", "Branch or ref to traverse")
	cmd.Flags().BoolVar(&cfg.IncludeMessages, "include-commit-messages", false, "Include commit messages in output")
	cmd.Flags().DurationVar(&cfg.CommandTimeout, "command-timeout", extract.DefaultCommandTimeout, "Maximum duration for git commands")
	cmd.Flags().BoolVar(&cfg.FirstParent, "first-parent", false, "Restrict to first-parent chain")
	cmd.Flags().IntVar(&cfg.DiscardWarnLimit, "discard-warn-limit", 20, "Max ignored entries before summarizing")
	cmd.Flags().BoolVar(&cfg.DiscardError, "discard-error", false, "Fail when ignored entries exceed warn limit")
	cmd.Flags().BoolVar(&cfg.Debug, "debug", false, "Enable verbose debug logging")

	return cmd
}

var validFormats = map[string]bool{"table": true, "csv": true, "json": true}
var validGranularities = map[string]bool{"day": true, "week": true, "month": true, "year": true}
var validStats = map[string]bool{
	"summary": true, "contributors": true, "hotspots": true,
	"activity": true, "busfactor": true,
}

func statsCmd() *cobra.Command {
	var (
		input       string
		format      string
		topN        int
		granularity string
		stat        string
	)

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Generate statistics from extracted JSONL data",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validFormats[format] {
				return fmt.Errorf("invalid --format %q; must be one of: table, csv, json", format)
			}
			if !validGranularities[granularity] {
				return fmt.Errorf("invalid --granularity %q; must be one of: day, week, month, year", granularity)
			}
			if stat != "" && !validStats[stat] {
				return fmt.Errorf("invalid --stat %q; must be one of: summary, contributors, hotspots, activity, busfactor", stat)
			}

			ds, err := stats.LoadJSONL(input)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Loaded %d commits, %d files, %d devs\n\n",
				len(ds.Commits), len(ds.Files), len(ds.Devs))

			showAll := stat == ""
			f := stats.NewFormatter(os.Stdout, format)

			if format == "json" {
				return printStatsJSON(f, ds, topN, granularity, stat)
			}

			if showAll || stat == "summary" {
				fmt.Fprintln(os.Stderr, "=== Summary ===")
				if err := f.PrintSummary(stats.ComputeSummary(ds)); err != nil {
					return err
				}
			}
			if showAll || stat == "contributors" {
				fmt.Fprintf(os.Stderr, "\n=== Top %d Contributors ===\n", topN)
				if err := f.PrintContributors(stats.TopContributors(ds, topN)); err != nil {
					return err
				}
			}
			if showAll || stat == "hotspots" {
				fmt.Fprintf(os.Stderr, "\n=== Top %d File Hotspots ===\n", topN)
				if err := f.PrintHotspots(stats.FileHotspots(ds, topN)); err != nil {
					return err
				}
			}
			if showAll || stat == "activity" {
				fmt.Fprintf(os.Stderr, "\n=== Activity (%s) ===\n", granularity)
				if err := f.PrintActivity(stats.ActivityOverTime(ds, granularity)); err != nil {
					return err
				}
			}
			if showAll || stat == "busfactor" {
				fmt.Fprintf(os.Stderr, "\n=== Top %d Bus Factor Risk ===\n", topN)
				if err := f.PrintBusFactor(stats.BusFactor(ds, topN)); err != nil {
					return err
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&input, "input", "git_data.jsonl", "Input JSONL file from extract")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, csv, json")
	cmd.Flags().IntVar(&topN, "top", 10, "Number of top entries to show")
	cmd.Flags().StringVar(&granularity, "granularity", "month", "Activity granularity: day, week, month, year")
	cmd.Flags().StringVar(&stat, "stat", "", "Show only a specific stat: summary, contributors, hotspots, activity, busfactor")

	return cmd
}

func printStatsJSON(f *stats.Formatter, ds *stats.Dataset, topN int, granularity, stat string) error {
	showAll := stat == ""
	report := make(map[string]interface{})

	if showAll || stat == "summary" {
		report["summary"] = stats.ComputeSummary(ds)
	}
	if showAll || stat == "contributors" {
		report["contributors"] = stats.TopContributors(ds, topN)
	}
	if showAll || stat == "hotspots" {
		report["hotspots"] = stats.FileHotspots(ds, topN)
	}
	if showAll || stat == "activity" {
		report["activity"] = stats.ActivityOverTime(ds, granularity)
	}
	if showAll || stat == "busfactor" {
		report["busfactor"] = stats.BusFactor(ds, topN)
	}

	return f.PrintReport(report)
}
