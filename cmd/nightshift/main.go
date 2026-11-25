// Command nightshift is the single entry point of the Nightshift factory.
package main

import (
	"os"

	// The images carry no time zone database, and tenants name their zone (schedule.timezone).
	_ "time/tzdata"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
