// Package app holds the logic for command line logic.
package app

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var errHelp = errors.New("help message printed")

func Run() int {
	if err := rootCmd().Execute(); err != nil {
		if err != errHelp {
			fmt.Fprintf(os.Stderr, "error: "+err.Error())
		}
		return 1
	}
	return 0
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "got",
		Short: "Got is a vendor directory manager.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Help()
			return nil
		},
	}
	return cmd
}
