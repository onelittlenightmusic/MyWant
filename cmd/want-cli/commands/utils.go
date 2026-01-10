package commands

import (
	"fmt"
	"os"
	"text/tabwriter"

	"mywant/pkg/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var LlmCmd = &cobra.Command{
	Use:   "llm",
	Short: "LLM utilities",
}

var queryLlmCmd = &cobra.Command{
	Use:   "query [prompt]",
	Short: "Query the LLM",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		model, _ := cmd.Flags().GetString("model")
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.QueryLLM(args[0], model)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(resp.Response)
	},
}

var LogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View system logs",
	Run: func(cmd *cobra.Command, args []string) {
		c := client.NewClient(viper.GetString("server"))
		resp, err := c.GetLogs()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Timestamp\tMethod\tEndpoint\tStatus\tDetails")

		for _, log := range resp.Logs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				log.Timestamp,
				log.Method,
				log.Endpoint,
				fmt.Sprintf("%s (%d)", log.Status, log.StatusCode),
				log.Details,
			)
		}
		w.Flush()
	},
}

func init() {
	LlmCmd.AddCommand(queryLlmCmd)
	queryLlmCmd.Flags().StringP("model", "m", "", "Model to use")
}
