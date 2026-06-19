package output

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"ci-cd/internal/runner"
)

func Format(cmd *cobra.Command, results []runner.Result, jsonOutput bool) error {
	if jsonOutput {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
	} else {
		for _, r := range results {
			status := "✅"
			if r.Status != "pass" {
				status = "❌"
			}
			fmt.Printf("[%s] %s (%s)\n", r.Project, status, r.Duration)
			for _, s := range r.Steps {
				stepStatus := "✅"
				if s.Status != "pass" {
					stepStatus = "❌"
				}
				fmt.Printf("  %s %s (%s)\n", stepStatus, s.Name, s.Duration)
			}
		}
	}
	return nil
}
