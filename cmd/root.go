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

var cfgFile string
var appConfig config.Config

var rootCmd = &cobra.Command{
	Use:   "watchdog",
	Short: "A balance monitoring watchdog",
	Long:  `Watchdog checks your Telnyx balance and notifies you via webhooks if it drops below a threshold.`,
	Run: func(cmd *cobra.Command, args []string) {
		runApp()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {

	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file: %s\n", err)
	}

	if err := viper.Unmarshal(&appConfig); err != nil {
		fmt.Printf("Unable to decode into struct: %v\n", err)
	}
}

func runApp() {
	// Initialize the scheduler
	sched := scheduler.NewScheduler()

	fmt.Printf("Loaded configuration using: %s\n", viper.ConfigFileUsed())
	fmt.Printf("Monitoring %s with threshold $%.2f\n", appConfig.Telnyx.APIURL, appConfig.Telnyx.Threshold)

	// Initialize the notifier
	notif := notifier.NewWebhookNotifier(appConfig.Notifier.AppriseAPIURL, appConfig.Notifier.GetServiceURLs())

	// Register the Telnyx balance check task
	task := tasks.NewTelnyxBalanceCheckTask(
		appConfig.Telnyx.APIURL,
		appConfig.Telnyx.APIKey,
		appConfig.Telnyx.Threshold,
		appConfig.Telnyx.GetNotificationCooldown(),
		notif,
	)

	// Schedule the task
	interval := appConfig.Scheduler.GetInterval()
	sched.ScheduleTask(task, interval)

	// Register and schedule GitHub PR review check task if configured
	if len(appConfig.GitHub.Repositories) > 0 {
		fmt.Printf("Monitoring %d GitHub repositories for stale PRs (threshold: %d days)\n",
			len(appConfig.GitHub.Repositories), appConfig.GitHub.GetStaleDays())

		prTask := tasks.NewPRReviewCheckTask(appConfig.GitHub, notif)
		sched.ScheduleTask(prTask, interval)
	}

	// Start the scheduler
	fmt.Printf("Starting scheduler with interval %v...\n", interval)
	sched.Start()

	// Keep the program running
	select {}
}
