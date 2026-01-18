package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"watchdog/internal/config"
	"watchdog/internal/notifier"
	"watchdog/internal/scheduler"
	"watchdog/tasks"
)

// cfgFile holds the path to the configuration file specified via command-line flag.
// If empty, the application will look for config.yaml in the current directory.
var cfgFile string

// appConfig stores the parsed configuration from the YAML file.
// This includes settings for Telnyx monitoring, GitHub PR monitoring, notifications, and scheduling.
var appConfig config.Config

// rootCmd represents the base command when called without any subcommands.
// It serves as the entry point for the Cobra CLI framework and executes the main application logic.
var rootCmd = &cobra.Command{
	Use:   "watchdog",
	Short: "A monitoring watchdog for Telnyx balance and GitHub PRs",
	Long: `Watchdog is a monitoring tool that:
  - Checks your Telnyx account balance and alerts when it drops below a threshold
  - Monitors GitHub pull requests and notifies when they're stale (pending review for too long)
  - Sends notifications via Apprise (supports Telegram, Discord, email, and more)`,
	Run: func(cmd *cobra.Command, args []string) {
		runApp()
	},
}

// Execute is the main entry point for the CLI application.
// It initializes the Cobra command structure and handles any errors that occur during execution.
// This function is called by main() and should only be invoked once.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// init is called automatically before main() and sets up the CLI flags and configuration.
// It registers the initConfig function to be called before command execution,
// and defines the --config flag for specifying a custom configuration file path.
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
}

// initConfig reads the configuration file and unmarshals it into the appConfig struct.
// It supports both explicit config file paths (via --config flag) and automatic discovery.
// If no config file is specified, it looks for config.yaml in the current directory.
// Environment variables are also automatically bound and can override config file values.
// This function will terminate the application with a fatal error if:
//   - The config file cannot be read
//   - The config file cannot be unmarshaled
//   - Required configuration fields are missing or invalid
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config.yaml in the current directory
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// Read environment variables that match config keys
	viper.AutomaticEnv()

	// Read the config file - this is fatal if it fails
	if err := viper.ReadInConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config file: %s\n", err)
		fmt.Fprintf(os.Stderr, "Please ensure a valid config file exists (use --config flag or create config.yaml)\n")
		os.Exit(1)
	}

	// Unmarshal the config into our struct - this is fatal if it fails
	if err := viper.Unmarshal(&appConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to decode config into struct: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please check your config file format matches the expected structure\n")
		os.Exit(1)
	}

	// Validate required configuration fields
	if err := validateConfig(&appConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}
}

// validateConfig checks that all required configuration fields are properly set.
// It returns an error if any critical fields are missing or invalid.
func validateConfig(cfg *config.Config) error {
	// Validate notifier configuration
	if cfg.Notifier.AppriseAPIURL == "" {
		return fmt.Errorf("notifier.apprise_api_url is required but not set")
	}
	if len(cfg.Notifier.GetServiceURLs()) == 0 {
		return fmt.Errorf("notifier.apprise_service_url is required but not set")
	}

	// Validate scheduler configuration
	if cfg.Scheduler.Interval == "" {
		return fmt.Errorf("scheduler.interval is required but not set")
	}

	// Validate Telnyx configuration if API URL is set
	if cfg.Tasks.Telnyx.APIURL != "" {
		if cfg.Tasks.Telnyx.APIKey == "" {
			return fmt.Errorf("tasks.telnyx.api_key is required when api_url is set")
		}
	}

	// Validate GitHub configuration if repositories are configured
	if len(cfg.Tasks.GitHub.Repositories) > 0 {
		for i, repo := range cfg.Tasks.GitHub.Repositories {
			if repo.Owner == "" {
				return fmt.Errorf("tasks.github.repositories[%d].owner is required", i)
			}
			if repo.Repo == "" {
				return fmt.Errorf("tasks.github.repositories[%d].repo is required", i)
			}
		}
	}

	return nil
}

// runApp is the main application logic that runs after CLI initialization.
// It performs the following steps:
//  1. Creates a scheduler to manage periodic tasks
//  2. Initializes the webhook notifier (Apprise) for sending alerts
//  3. Sets up the Telnyx balance check task (if configured)
//  4. Sets up the GitHub PR review check task (if repositories are configured)
//  5. Starts the scheduler and keeps the application running indefinitely
//
// The application will continue running until manually stopped (Ctrl+C) or killed.
func runApp() {
	// Initialize the scheduler that will run our tasks periodically
	sched := scheduler.NewScheduler()

	fmt.Printf("Loaded configuration using: %s\n", viper.ConfigFileUsed())

	// Get global default interval from scheduler config
	globalInterval := appConfig.Scheduler.GetInterval()
	fmt.Printf("Global scheduler interval: %v\n", globalInterval)

	// Initialize the notifier - this handles sending alerts via Apprise
	// Apprise supports multiple notification services (Telegram, Discord, email, etc.)
	notif := notifier.NewWebhookNotifier(appConfig.Notifier.AppriseAPIURL, appConfig.Notifier.GetServiceURLs())

	// Register the Telnyx balance check task
	// This task periodically checks your Telnyx account balance and sends an alert
	// if it falls below the configured threshold
	telnyxCfg := appConfig.Tasks.Telnyx
	task := tasks.NewTelnyxBalanceCheckTask(
		telnyxCfg.APIURL,
		telnyxCfg.APIKey,
		telnyxCfg.Threshold,
		telnyxCfg.GetNotificationCooldown(),
		notif,
	)

	// Schedule the task with per-task interval (falls back to global if not set)
	telnyxInterval := telnyxCfg.GetInterval(globalInterval)
	fmt.Printf("Monitoring Telnyx balance at %s with threshold $%.2f (interval: %v)\n",
		telnyxCfg.APIURL, telnyxCfg.Threshold, telnyxInterval)
	sched.ScheduleTask(task, telnyxInterval)

	// Register and schedule GitHub PR review check task if repositories are configured
	// This task monitors GitHub PRs and alerts when they've been pending review for too long
	githubCfg := appConfig.Tasks.GitHub
	if len(githubCfg.Repositories) > 0 {
		githubInterval := githubCfg.GetInterval(globalInterval)
		fmt.Printf("Monitoring %d GitHub repositories for stale PRs (threshold: %d days, interval: %v)\n",
			len(githubCfg.Repositories), githubCfg.GetStaleDays(), githubInterval)

		prTask := tasks.NewPRReviewCheckTask(githubCfg, notif)
		sched.ScheduleTask(prTask, githubInterval)
	}

	// Start the scheduler - this begins executing all registered tasks
	fmt.Println("Starting scheduler...")
	sched.Start()

	// Keep the program running indefinitely
	// The select{} statement blocks forever, allowing the scheduler goroutines to continue
	select {}
}
