package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize the global logger with pretty console output
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	},
	Run: func(cmd *cobra.Command, args []string) {
		runApp()
	},
}

// Execute is the main entry point for the CLI application.
// It initializes the Cobra command structure and handles any errors that occur during execution.
// Execute runs the root CLI command and exits the process with status 1 if command execution fails.
// It is intended to be invoked once from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// init is called automatically before main() and sets up the CLI flags and configuration.
// It registers the initConfig function to be called before command execution,
// init registers initConfig to run on Cobra initialization and defines the
// persistent --config flag to specify a custom configuration file path
// (default ./config.yaml).
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
//
// initConfig reads configuration from the file specified by the --config flag (or config.yaml in the current directory) and environment variables, unmarshals it into the package-level appConfig, and validates required fields.
// On read, unmarshal, or validation failure it writes an error message to stderr and exits the process with status 1.
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
// validateConfig verifies required configuration fields for notifier, scheduler,
// Telnyx, and GitHub.
// It returns an error describing the first missing or invalid field, or nil if all checks pass.
// Conditional checks:
//   - Telnyx fields are validated only when Tasks.Telnyx.APIURL is set.
//   - Each GitHub repository must include both Owner and Repo when any repositories are configured.
func validateConfig(cfg *config.Config) error {
	// Validate notifier configuration
	if cfg.Notifier.AppriseAPIURL == "" {
		return fmt.Errorf("notifier.apprise_api_url is required but not set")
	}
	if len(cfg.Notifier.GetServiceURLs()) == 0 {
		return fmt.Errorf("notifier.apprise_service_url is required but not set")
	}

	// Validate scheduler configuration
	// Note: Config.Scheduler.Interval is allowed to be empty;
	// SchedulerConfig.GetInterval() will provide a default (5m) in that case.

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
// runApp initializes the scheduler and notifier, registers configured tasks (Telnyx balance checks and GitHub PR review checks), starts periodic execution, and waits for a termination signal to perform a graceful shutdown.
// It prints runtime status to stdout and exits with status 1 if no tasks are configured.
func runApp() {
	// Initialize the scheduler that will run our tasks periodically
	sched := scheduler.NewScheduler()

	log.Info().Str("config_file", viper.ConfigFileUsed()).Msg("Configuration loaded")

	// Get global default interval from scheduler config
	globalInterval := appConfig.Scheduler.GetInterval()
	log.Info().Dur("global_interval", globalInterval).Msg("Global scheduler interval set")

	// Initialize the notifier - this handles sending alerts via Apprise
	// Apprise supports multiple notification services (Telegram, Discord, email, etc.)
	notif := notifier.NewWebhookNotifier(appConfig.Notifier.AppriseAPIURL, appConfig.Notifier.GetServiceURLs())

	// Register the Telnyx balance check task (if configured)
	// This task periodically checks your Telnyx account balance and sends an alert
	// if it falls below the configured threshold
	telnyxCfg := appConfig.Tasks.Telnyx
	if telnyxCfg.APIURL != "" && telnyxCfg.APIKey != "" {
		telnyxInterval := telnyxCfg.GetInterval(globalInterval)
		log.Info().
			Str("api_url", telnyxCfg.APIURL).
			Float64("threshold", telnyxCfg.Threshold).
			Dur("interval", telnyxInterval).
			Msg("Telnyx monitoring enabled")

		task := tasks.NewTelnyxBalanceCheckTask(
			telnyxCfg.APIURL,
			telnyxCfg.APIKey,
			telnyxCfg.Threshold,
			telnyxCfg.GetNotificationCooldown(),
			notif,
		)
		sched.ScheduleTask(task, telnyxInterval)
	} else {
		log.Info().Msg("Telnyx monitoring disabled (api_url or api_key not configured)")
	}

	// Register and schedule GitHub PR review check task if repositories are configured
	// This task monitors GitHub PRs and alerts when they've been pending review for too long
	githubCfg := appConfig.Tasks.GitHub
	if len(githubCfg.Repositories) > 0 {
		githubInterval := githubCfg.GetInterval(globalInterval)
		log.Info().
			Int("repository_count", len(githubCfg.Repositories)).
			Int("stale_threshold_days", githubCfg.GetStaleDays()).
			Dur("interval", githubInterval).
			Msg("GitHub monitoring enabled")

		prTask := tasks.NewPRReviewCheckTask(githubCfg, notif)
		sched.ScheduleTask(prTask, githubInterval)
	} else {
		log.Info().Msg("GitHub monitoring disabled (no repositories configured)")
	}

	// Check if at least one task was scheduled
	if !sched.HasTasks() {
		log.Fatal().Msg("No tasks configured! Please configure at least one of: Telnyx monitoring or GitHub monitoring")
	}

	// Start the scheduler - this begins executing all registered tasks
	log.Info().Msg("Starting scheduler...")
	sched.Start()

	// Wait for interrupt signal for graceful shutdown
	// This allows the program to be stopped cleanly with Ctrl+C (SIGINT) or kill (SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info().Msg("Watchdog is running. Press Ctrl+C to stop.")
	<-sigChan

	// Graceful shutdown
	log.Info().Msg("Shutting down gracefully...")
	sched.Stop()
	log.Info().Msg("Shutdown complete.")
}
