package scheduler

import (
	"fmt"
	"time"
)

type Task interface {
	Run() error
}

type Scheduler struct {
	tasks []*scheduledTask
}

type scheduledTask struct {
	task     Task
	interval time.Duration
	stop     chan bool
}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) ScheduleTask(task Task, interval time.Duration) {
	scheduledTask := &scheduledTask{
		task:     task,
		interval: interval,
		stop:     make(chan bool),
	}
	s.tasks = append(s.tasks, scheduledTask)
}

func (s *Scheduler) Start() {
	for _, st := range s.tasks {
		go func(task *scheduledTask) {
			ticker := time.NewTicker(task.interval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					err := task.task.Run()
					if err != nil {
						fmt.Printf("Error running task: %v\n", err)
					}
				case <-task.stop:
					return
				}
			}
		}(st)
	}
}

func (s *Scheduler) Stop() {
	for _, scheduledTask := range s.tasks {
		scheduledTask.stop <- true
	}
}
