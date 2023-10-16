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

	// outputCRFile specifies the location of the generated CredentialsRequest YAML.
	outputCRFile string

	// pkg specifies the package with which the code is generated.
	pkg string

	// skipMinify specifies whether the minification of the AWS policy has to be skipped.
	skipMinify bool

	// splitResource splits IAM policy's statement into many with one resource per statement.
	splitResource bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "iamctl",
	Short: "A CLI used to convert aws iam policy JSON to Go code and other formats.",
	Long: `A CLI used to convert aws iam policy JSON to Go code. This
	CLI produces a '.go' file that is consumed by the aws load balancer operator.
    Also it can produce a CredentialsRequest YAML file which can provision the secret for the controller.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		generateIAMPolicy(inputFile, outputFile, outputCRFile, pkg)
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
	_ = rootCmd.MarkPersistentFlagRequired("input-file")

	rootCmd.PersistentFlags().StringVarP(&outputFile, "output-file", "o", "", "Used to specify output Go file path.")
	_ = rootCmd.MarkPersistentFlagRequired("output-file")

	rootCmd.PersistentFlags().StringVarP(&outputCRFile, "output-cr-file", "c", "", "Used to specify output CredentialsRequest YAML file path.")

	rootCmd.PersistentFlags().StringVarP(&pkg, "package", "p", "main", "Used to specify output Go file path.")
	_ = rootCmd.MarkPersistentFlagRequired("package")

	rootCmd.PersistentFlags().BoolVarP(&skipMinify, "no-minify", "n", false, "Used to skip the minification of the output AWS policy.")

	rootCmd.PersistentFlags().BoolVarP(&splitResource, "split-resource", "s", false, "Used to split AWS policy's statement into many with one resource per statement.")
}
