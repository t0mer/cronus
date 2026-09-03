// Command cronus is an NTP server tester and comparator: it queries multiple
// NTP servers, compares their responses side by side, and (in monitoring mode)
// tracks clock offset and drift over time.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
