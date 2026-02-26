// Package cmd implements the command-line interface for pg_rocket using Cobra.
// It defines the root command and all subcommands (pull, inspect, version).
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current version of pg_rocket, set at build time via ldflags.
var Version = "0.0.1"

var rootCmd = &cobra.Command{
	Use:   "pg_rocket",
	Short: "Extract referentially complete PostgreSQL data subsets",
	Long: `pg_rocket is a CLI tool that extracts referentially complete subsets of data
from PostgreSQL databases by traversing foreign key relationships.`,
}

// Execute runs the root command and returns any error encountered.
// This is called from main.go.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("pg_rocket v%s\n", Version)
	},
}
