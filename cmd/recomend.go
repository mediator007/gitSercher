package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var RecommendCmd = &cobra.Command{
	Use:   "recommend-workers",
	Short: "Recommend optimal number of workers",
	Long:  "Suggests optimal number of workers based on system CPU",
	Run: func(cmd *cobra.Command, args []string) {
		cpu := runtime.NumCPU()

		conservative := cpu * 2
		aggressive := cpu * 4

		fmt.Println("System info:")
		fmt.Printf("CPU cores: %d\n\n", cpu)

		fmt.Println("Recommended workers:")
		fmt.Printf("Conservative (safe): %d\n", conservative)
		fmt.Printf("Aggressive (fast):   %d\n", aggressive)

		fmt.Println("\nTips:")
		fmt.Println("- Use lower values (10–30) for HDD or heavy systems")
		fmt.Println("- Use higher values (50–200) for SSD/NVMe")
		fmt.Println("- If you see 'signal: killed' → reduce workers")
	},
}
