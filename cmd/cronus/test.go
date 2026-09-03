package main

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/t0mer/cronus/internal/ntp"
)

func newTestCmd(cfgFile *string) *cobra.Command {
	var (
		samples  int
		asJSON   bool
		showDelt bool
	)
	cmd := &cobra.Command{
		Use:   "test <server> [server...]",
		Short: "Query and compare one or more NTP servers once",
		Long: "Runs an on-demand test against the given NTP servers " +
			"(host or host:port) and prints a side-by-side comparison.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd, *cfgFile)
			if err != nil {
				return err
			}
			setupLogger(cfg.Log.Level)
			if cmd.Flags().Changed("samples") {
				cfg.NTP.Samples = samples
			}
			engine := buildEngine(cfg)

			results := engine.Run(context.Background(), args)
			comp := ntp.BuildComparison(results, cfg.Compare.OutlierThreshold)

			if asJSON {
				return printJSON(cmd, results, comp)
			}
			printTable(cmd, results, comp, showDelt)
			return nil
		},
	}
	cmd.Flags().IntVar(&samples, "samples", 4, "samples per server (1-10)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	cmd.Flags().BoolVar(&showDelt, "deltas", false, "show the pairwise offset-delta matrix")
	return cmd
}

type testOutput struct {
	Results    []ntp.ServerResult `json:"results"`
	Comparison ntp.Comparison     `json:"comparison"`
}

func printJSON(cmd *cobra.Command, results []ntp.ServerResult, comp ntp.Comparison) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(testOutput{Results: results, Comparison: comp})
}

func printTable(cmd *cobra.Command, results []ntp.ServerResult, comp ntp.Comparison, showDeltas bool) {
	out := cmd.OutOrStdout()
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SERVER\tRESOLVED\tREACH\tOFFSET\tRTT\tJITTER\tSTRATUM\tREFID\tLEAP")
	for _, r := range results {
		reach := "yes"
		if !r.Reachable {
			reach = "no"
		}
		if !r.Reachable {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t\t\t\t\t\n", r.Target, r.ResolvedIP, reach, "("+shorten(r.Error)+")")
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\n",
			r.Target, r.ResolvedIP, reach,
			dur(r.Offset), dur(r.RTT), dur(r.Jitter),
			r.Stratum, r.ReferenceID, r.Leap)
	}
	tw.Flush()

	if len(comp.Labels) > 0 {
		fmt.Fprintf(out, "\nConsensus (median offset): %s\n", dur(comp.MedianOffset))
		if len(comp.Outliers) > 0 {
			fmt.Fprintf(out, "Suspected falsetickers: %v\n", comp.Outliers)
		} else {
			fmt.Fprintln(out, "Suspected falsetickers: none")
		}
	}

	if showDeltas && len(comp.Labels) > 1 {
		fmt.Fprintln(out, "\nPairwise offset deltas (row - col):")
		dtw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		fmt.Fprint(dtw, "\t")
		for _, l := range comp.Labels {
			fmt.Fprintf(dtw, "%s\t", l)
		}
		fmt.Fprintln(dtw)
		for i, l := range comp.Labels {
			fmt.Fprintf(dtw, "%s\t", l)
			for j := range comp.Labels {
				fmt.Fprintf(dtw, "%s\t", dur(comp.Pairwise[i][j]))
			}
			fmt.Fprintln(dtw)
		}
		dtw.Flush()
	}
}

func dur(d time.Duration) string {
	return d.Round(time.Microsecond).String()
}

func shorten(s string) string {
	const max = 40
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}
