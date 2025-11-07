package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kadirbelkuyu/DBRTS/internal/app"
	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/ui/explorer"

	"github.com/spf13/cobra"
)

const appName = "Database Backup Restore Transfer System"

const asciiBanner = `
                                                                                                                
 ██████████   ███████████  ███████████   ███████████  █████████ 
░░███░░░░███ ░░███░░░░░███░░███░░░░░███ ░█░░░███░░░█ ███░░░░░███
 ░███   ░░███ ░███    ░███ ░███    ░███ ░   ░███  ░ ░███    ░░░ 
 ░███    ░███ ░██████████  ░██████████      ░███    ░░█████████ 
 ░███    ░███ ░███░░░░░███ ░███░░░░░███     ░███     ░░░░░░░░███
 ░███    ███  ░███    ░███ ░███    ░███     ░███     ███    ░███
 ██████████   ███████████  █████   █████    █████   ░░█████████ 
░░░░░░░░░░   ░░░░░░░░░░░  ░░░░░   ░░░░░    ░░░░░     ░░░░░░░░░                                                             
                                                                                                                
`

var rootCmd = &cobra.Command{
	Use:   "dbrts",
	Short: "Unified dbrts toolkit for PostgreSQL and MongoDB",
	Long:  `A developer-friendly CLI to transfer data, create backups, restore archives, and inspect PostgreSQL or MongoDB databases.`,
	RunE:  runInteractive,
}

var transferCmd = &cobra.Command{
	Use:   "transfer",
	Short: "Start a data transfer between databases",
	RunE:  runTransfer,
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a database backup",
	RunE:  runBackup,
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore a database backup",
	RunE:  runRestore,
}

var listDbCmd = &cobra.Command{
	Use:   "list-databases",
	Short: "List databases available on the server",
	RunE:  runListDatabases,
}

var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Launch the guided interactive workflow",
	RunE:  runInteractive,
}

var exploreCmd = &cobra.Command{
	Use:   "explore",
	Short: "Inspect databases with the interactive console",
	RunE:  runExplore,
}

var workflowService = app.NewService()

var (
	sourceConfigPath string
	targetConfigPath string
	configPath       string
	schemaOnly       bool
	dataOnly         bool
	parallelWorkers  int
	batchSize        int
	verbose          bool
)

func init() {
	transferCmd.Flags().StringVar(&sourceConfigPath, "source-config", "", "Path to the source database configuration file")
	transferCmd.Flags().StringVar(&targetConfigPath, "target-config", "", "Path to the target database configuration file")
	transferCmd.Flags().BoolVar(&schemaOnly, "schema-only", false, "Transfer schema objects only")
	transferCmd.Flags().BoolVar(&dataOnly, "data-only", false, "Transfer data only")
	transferCmd.Flags().IntVar(&parallelWorkers, "workers", 4, "Number of parallel workers during transfer")
	transferCmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Batch size for data transfer")
	transferCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")

	transferCmd.MarkFlagRequired("source-config")
	transferCmd.MarkFlagRequired("target-config")

	backupCmd.Flags().StringVar(&configPath, "config", "", "Path to the database configuration file")
	backupCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	backupCmd.MarkFlagRequired("config")

	restoreCmd.Flags().StringVar(&configPath, "config", "", "Path to the database configuration file")
	restoreCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	restoreCmd.MarkFlagRequired("config")

	listDbCmd.Flags().StringVar(&configPath, "config", "", "Path to the database configuration file")
	listDbCmd.MarkFlagRequired("config")

	exploreCmd.Flags().StringVar(&configPath, "config", "", "Path to the database configuration file")
	exploreCmd.MarkFlagRequired("config")

	rootCmd.AddCommand(transferCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(listDbCmd)
	rootCmd.AddCommand(interactiveCmd)
	rootCmd.AddCommand(exploreCmd)

	cobra.OnInitialize(func() {
		rootCmd.SilenceUsage = true
		rootCmd.SilenceErrors = true
	})
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runInteractive(cmd *cobra.Command, args []string) error {
	application := app.NewApplication(os.Stdin, printBanner)
	return application.RunInteractive()
}

func runTransfer(cmd *cobra.Command, args []string) error {
	sourceConfig, err := config.LoadConfig(sourceConfigPath)
	if err != nil {
		return fmt.Errorf("cannot load source config: %w", err)
	}

	targetConfig, err := config.LoadConfig(targetConfigPath)
	if err != nil {
		return fmt.Errorf("cannot load target config: %w", err)
	}

	return workflowService.Transfer(sourceConfig, targetConfig, schemaOnly, dataOnly, parallelWorkers, batchSize, verbose)
}

func runBackup(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("cannot load config: %w", err)
	}

	return workflowService.Backup(cfg, verbose)
}

func runRestore(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("cannot load config: %w", err)
	}

	return workflowService.Restore(cfg, verbose)
}

func runListDatabases(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("cannot load config: %w", err)
	}

	return workflowService.ListDatabases(cfg)
}

func runExplore(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("cannot load config: %w", err)
	}
	return explorer.Run(cfg)
}

func printBanner() {
	fmt.Print(asciiBanner)
	fmt.Println(appName)
	fmt.Println(strings.Repeat("-", len(appName)))
}
