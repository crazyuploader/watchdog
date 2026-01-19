package scheduler

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTask is a mock implementation of the Task interface for testing
type MockTask struct {
	runCount   int
	runError   error
	runFunc    func() error
	mu         sync.Mutex
	runHistory []time.Time
}

func (m *MockTask) Run() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.runCount++
	m.runHistory = append(m.runHistory, time.Now())

	if m.runFunc != nil {
		return m.runFunc()
	}
	return m.runError
}

func (m *MockTask) GetRunCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runCount
}

func (m *MockTask) GetRunHistory() []time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Time{}, m.runHistory...)
}

func TestNewScheduler(t *testing.T) {
	sched := NewScheduler()

	assert.NotNil(t, sched)
	assert.NotNil(t, sched.tasks)
	assert.Empty(t, sched.tasks)
}

func TestScheduler_ScheduleTask(t *testing.T) {
	sched := NewScheduler()
	task := &MockTask{}

	sched.ScheduleTask(task, 5*time.Minute)

	assert.Len(t, sched.tasks, 1)
	assert.Equal(t, task, sched.tasks[0].task)
	assert.Equal(t, 5*time.Minute, sched.tasks[0].interval)
	assert.NotNil(t, sched.tasks[0].stop)
}

func TestScheduler_ScheduleMultipleTasks(t *testing.T) {
	sched := NewScheduler()
	task1 := &MockTask{}
	task2 := &MockTask{}
	task3 := &MockTask{}

	sched.ScheduleTask(task1, 5*time.Minute)
	sched.ScheduleTask(task2, 10*time.Minute)
	sched.ScheduleTask(task3, 15*time.Minute)

	assert.Len(t, sched.tasks, 3)
	assert.Equal(t, 5*time.Minute, sched.tasks[0].interval)
	assert.Equal(t, 10*time.Minute, sched.tasks[1].interval)
	assert.Equal(t, 15*time.Minute, sched.tasks[2].interval)
}

func TestScheduler_HasTasks(t *testing.T) {
	sched := NewScheduler()
	assert.False(t, sched.HasTasks())

	task := &MockTask{}
	sched.ScheduleTask(task, 5*time.Minute)
	assert.True(t, sched.HasTasks())
}

func TestScheduler_Start_ExecutesTasks(t *testing.T) {
	sched := NewScheduler()
	task := &MockTask{}

	// Use a very short interval for testing
	sched.ScheduleTask(task, 50*time.Millisecond)
	sched.Start()

	// Wait for multiple executions
	time.Sleep(250 * time.Millisecond)

	runCount := task.GetRunCount()
	assert.Greater(t, runCount, 1, "Task should have run multiple times")
	assert.LessOrEqual(t, runCount, 6, "Task shouldn't run too many times")

	sched.Stop()
}

func TestScheduler_Start_MultipleTasksRunIndependently(t *testing.T) {
	sched := NewScheduler()
	task1 := &MockTask{}
	task2 := &MockTask{}

	// Different intervals
	sched.ScheduleTask(task1, 50*time.Millisecond)
	sched.ScheduleTask(task2, 100*time.Millisecond)
	sched.Start()

	time.Sleep(250 * time.Millisecond)

	count1 := task1.GetRunCount()
	count2 := task2.GetRunCount()

	assert.Greater(t, count1, 0)
	assert.Greater(t, count2, 0)
	// task1 runs twice as often as task2
	assert.Greater(t, count1, count2)

	sched.Stop()
}

func TestScheduler_Start_TaskErrorsAreLogged(t *testing.T) {
	sched := NewScheduler()
	task := &MockTask{
		runError: errors.New("task failed"),
	}

	sched.ScheduleTask(task, 50*time.Millisecond)
	sched.Start()

	time.Sleep(150 * time.Millisecond)

	// Task should continue running despite errors
	assert.Greater(t, task.GetRunCount(), 1)

	sched.Stop()
}

func TestScheduler_Stop(t *testing.T) {
	sched := NewScheduler()
	task := &MockTask{}

	sched.ScheduleTask(task, 50*time.Millisecond)
	sched.Start()

	time.Sleep(100 * time.Millisecond)
	sched.Stop()

	countBeforeStop := task.GetRunCount()
	time.Sleep(150 * time.Millisecond)
	countAfterStop := task.GetRunCount()

	// Count should not increase after stop
	assert.Equal(t, countBeforeStop, countAfterStop)
}

func TestScheduler_Stop_MultipleTasks(t *testing.T) {
	sched := NewScheduler()
	task1 := &MockTask{}
	task2 := &MockTask{}

	sched.ScheduleTask(task1, 50*time.Millisecond)
	sched.ScheduleTask(task2, 50*time.Millisecond)
	sched.Start()

	time.Sleep(100 * time.Millisecond)
	sched.Stop()

	count1Before := task1.GetRunCount()
	count2Before := task2.GetRunCount()

	time.Sleep(150 * time.Millisecond)

	assert.Equal(t, count1Before, task1.GetRunCount())
	assert.Equal(t, count2Before, task2.GetRunCount())
}

func TestScheduler_Start_WithZeroTasks(t *testing.T) {
	sched := NewScheduler()

	// Should not panic
	assert.NotPanics(t, func() {
		sched.Start()
		time.Sleep(50 * time.Millisecond)
		sched.Stop()
	})
}

func TestScheduler_TaskRunsAtCorrectInterval(t *testing.T) {
	sched := NewScheduler()
	task := &MockTask{}

	interval := 100 * time.Millisecond
	sched.ScheduleTask(task, interval)
	sched.Start()

	time.Sleep(350 * time.Millisecond)
	sched.Stop()

	runHistory := task.GetRunHistory()
	require.GreaterOrEqual(t, len(runHistory), 2, "Need at least 2 runs to check interval")

	// Check intervals between runs
	for i := 1; i < len(runHistory); i++ {
		actualInterval := runHistory[i].Sub(runHistory[i-1])
		// Allow 20ms tolerance for timer precision
		assert.InDelta(t, interval.Milliseconds(), actualInterval.Milliseconds(), 20)
	}
}

func TestScheduler_TaskWithLongExecution(t *testing.T) {
	sched := NewScheduler()
	task := &MockTask{
		runFunc: func() error {
			time.Sleep(150 * time.Millisecond)
			return nil
		},
	}

	// Interval shorter than execution time
	sched.ScheduleTask(task, 50*time.Millisecond)
	sched.Start()

	time.Sleep(400 * time.Millisecond)
	sched.Stop()

	// Task should have run at least once despite long execution
	assert.GreaterOrEqual(t, task.GetRunCount(), 1)
	// But shouldn't run as many times as with a quick task
	assert.LessOrEqual(t, task.GetRunCount(), 3)
}

func TestScheduledTask_StopChannel(t *testing.T) {
	task := &MockTask{}
	st := &scheduledTask{
		task:     task,
		interval: 1 * time.Second,
		stop:     make(chan struct{}),
	}

	assert.NotNil(t, st.stop)
	assert.Equal(t, task, st.task)
	assert.Equal(t, 1*time.Second, st.interval)
}

func TestScheduler_ConcurrentTaskExecution(t *testing.T) {
	sched := NewScheduler()
	executionOrder := make([]int, 0)
	mu := sync.Mutex{}

	task1 := &MockTask{
		runFunc: func() error {
			mu.Lock()
			executionOrder = append(executionOrder, 1)
			mu.Unlock()
			return nil
		},
	}

	task2 := &MockTask{
		runFunc: func() error {
			mu.Lock()
			executionOrder = append(executionOrder, 2)
			mu.Unlock()
			return nil
		},
	}

	sched.ScheduleTask(task1, 50*time.Millisecond)
	sched.ScheduleTask(task2, 50*time.Millisecond)
	sched.Start()

	time.Sleep(200 * time.Millisecond)
	sched.Stop()

	mu.Lock()
	defer mu.Unlock()

	// Both tasks should have run
	assert.Greater(t, len(executionOrder), 2)
	// Verify both task IDs appear
	hasTask1 := false
	hasTask2 := false
	for _, id := range executionOrder {
		if id == 1 {
			hasTask1 = true
		}
		if id == 2 {
			hasTask2 = true
		}
	}
	assert.True(t, hasTask1)
	assert.True(t, hasTask2)
}

func TestScheduler_RestartAfterStop(t *testing.T) {
	sched := NewScheduler()
	task := &MockTask{}

	sched.ScheduleTask(task, 50*time.Millisecond)

	// First run
	sched.Start()
	time.Sleep(100 * time.Millisecond)
	sched.Stop()

	firstRunCount := task.GetRunCount()
	assert.Greater(t, firstRunCount, 0)

	// Wait to ensure stopped
	time.Sleep(100 * time.Millisecond)
	countAfterStop := task.GetRunCount()
	assert.Equal(t, firstRunCount, countAfterStop)

	// Note: Current implementation doesn't support restart
	// This test documents the expected behavior
}
