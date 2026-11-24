// Command nightshift is the single entry point of the Nightshift factory.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
