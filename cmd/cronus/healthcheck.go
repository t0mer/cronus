package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// newHealthcheckCmd provides a dependency-free liveness probe suitable for a
// container HEALTHCHECK on a scratch image (which has no curl/wget). It GETs
// /healthz on the local server and exits non-zero on failure.
func newHealthcheckCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:    "healthcheck",
		Short:  "Probe the local server's /healthz endpoint (for container health)",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			url := fmt.Sprintf("http://%s/healthz", addr)
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				fmt.Fprintf(os.Stderr, "unhealthy: %s\n", resp.Status)
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "server address to probe")
	return cmd
}
