package scheduler

import (
	"fmt"
	"time"
)

// Task defines the interface that all schedulable tasks must implement.
// Any struct that implements the Run() method can be scheduled for periodic execution.
//
// Examples of tasks in watchdog:
//   - TelnyxBalanceCheckTask: Checks Telnyx account balance
//   - PRReviewCheckTask: Monitors GitHub PRs for staleness
type Task interface {
	// Run executes the task logic.
	// It should return an error if the task fails, nil on success.
	// Errors are logged but don't stop the scheduler from continuing.
	Run() error
}

// Scheduler manages the periodic execution of multiple tasks.
// It runs each task in its own goroutine at the specified interval.
// Tasks continue running until the scheduler is stopped or the program exits.
//
// The scheduler uses Go's time.Ticker for precise interval timing.
type Scheduler struct {
	// tasks is the list of all scheduled tasks
	// Each task runs independently in its own goroutine
	tasks []*scheduledTask
}

// scheduledTask is an internal struct that wraps a Task with its scheduling metadata.
// It's not exported because users don't need to interact with it directly.
type scheduledTask struct {
	// task is the actual task to execute
	task Task

	// interval is how often to run the task (e.g., 5 minutes)
	interval time.Duration

	// stop is a channel used to signal the task goroutine to stop
	// Sending true on this channel will terminate the task's execution loop
	stop chan bool
}

// NewScheduler creates a new empty scheduler.
// Tasks must be added via ScheduleTask() before calling Start().
//
// Example:
//
//	sched := NewScheduler()
//	sched.ScheduleTask(myTask, 5*time.Minute)
//
// NewScheduler creates a new Scheduler initialized with no scheduled tasks.
func NewScheduler() *Scheduler {
	return &Scheduler{
		tasks: make([]*scheduledTask, 0),
	}
}

// ScheduleTask adds a task to the scheduler with the specified execution interval.
// The task won't start running until Start() is called.
//
// Parameters:
//   - task: The task to schedule (must implement the Task interface)
//   - interval: How often to run the task (e.g., 5*time.Minute, 1*time.Hour)
//
// Multiple tasks can be scheduled with different intervals.
// Each task runs independently in its own goroutine.
//
// Example:
//
//	sched.ScheduleTask(balanceTask, 5*time.Minute)  // Check balance every 5 minutes
//	sched.ScheduleTask(prTask, 10*time.Minute)      // Check PRs every 10 minutes
func (s *Scheduler) ScheduleTask(task Task, interval time.Duration) {
	scheduledTask := &scheduledTask{
		task:     task,
		interval: interval,
		stop:     make(chan bool),
	}
	s.tasks = append(s.tasks, scheduledTask)
}

// HasTasks returns true if at least one task has been scheduled.
// This is useful for checking if the scheduler has any work to do before starting it.
func (s *Scheduler) HasTasks() bool {
	return len(s.tasks) > 0
}

// Start begins executing all scheduled tasks.
// Each task runs in its own goroutine and executes at its configured interval.
//
// How it works:
//  1. For each scheduled task, a goroutine is spawned
//  2. Each goroutine creates a ticker that fires at the task's interval
//  3. When the ticker fires, the task's Run() method is called
//  4. If Run() returns an error, it's logged but execution continues
//  5. The goroutine continues until Stop() is called or the program exits
//
// This method returns immediately after starting all goroutines.
// The tasks will continue running in the background.
//
// Note: If a task's Run() method takes longer than the interval,
// the next execution will be delayed (tickers don't queue up).
func (s *Scheduler) Start() {
	for _, st := range s.tasks {
		// Launch each task in its own goroutine
		// We pass 'st' as a parameter to avoid closure issues
		go func(task *scheduledTask) {
			// Create a ticker that fires at the specified interval
			ticker := time.NewTicker(task.interval)
			defer ticker.Stop()

			// Infinite loop - runs until we receive a stop signal
			for {
				select {
				case <-ticker.C:
					// Ticker fired - time to run the task
					err := task.task.Run()
					if err != nil {
						// Log the error but continue running
						// We don't want one task failure to stop the scheduler
						fmt.Printf("Error running task: %v\n", err)
					}
				case <-task.stop:
					// Stop signal received - exit the goroutine
					return
				}
			}
		}(st)
	}
}

// Stop halts all running tasks.
// It sends a stop signal to each task's goroutine, causing them to exit.
//
// This is a graceful shutdown - it doesn't forcefully kill goroutines,
// but rather signals them to stop. If a task is currently executing,
// it will finish its current run before stopping.
//
// Note: In the current watchdog implementation, this method is never called
// because the main program runs indefinitely (select{} blocks forever).
// It's included for completeness and potential future use.
func (s *Scheduler) Stop() {
	for _, scheduledTask := range s.tasks {
		scheduledTask.stop <- true
	}
}
