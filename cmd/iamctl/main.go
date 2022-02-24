package main

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	// input file specifies the location for the input json.
	inputFile string

	// outputfile specifies the location of the generated code.
	outputFile string

	// pkg specifies the package with which the code is generated.
	pkg string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "iamctl",
	Short: "A CLI used to convert aws iam policy JSON to Go code.",
	Long: `A CLI used to convert aws iam policy JSON to Go code. This
	CLI produces a '.go' file that is consumed by the aws load balancer operator.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		generateIAMPolicy(inputFile, outputFile, pkg)
	},
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.PersistentFlags().StringVarP(&inputFile, "input-file", "i", "", "Used to specify input JSON file path.")
	rootCmd.MarkPersistentFlagRequired("input-file")

	rootCmd.PersistentFlags().StringVarP(&outputFile, "output-file", "o", "", "Used to specify output Go file path.")
	rootCmd.MarkPersistentFlagRequired("output-file")

	rootCmd.PersistentFlags().StringVarP(&pkg, "package", "p", "main", "Used to specify output Go file path.")
	rootCmd.MarkPersistentFlagRequired("package")
}
