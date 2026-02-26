// pg_rocket is a CLI tool for extracting referentially complete data subsets
// from PostgreSQL databases by traversing foreign key relationships.
//
// See README.md for usage documentation.
package main

import (
	"fmt"
	"os"

	"github.com/riyasyash/pg_rocket/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
