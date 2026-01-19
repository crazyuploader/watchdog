package tasks

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockTelnyxAPI mocks the Telnyx API client
type MockTelnyxClient struct {
	mock.Mock
}

func (m *MockTelnyxClient) GetBalance() (float64, error) {
	args := m.Called()
	return args.Get(0).(float64), args.Error(1)
}

// MockNotifier mocks the notification interface
type MockNotifier struct {
	mock.Mock
}

func (m *MockNotifier) SendNotification(subject, message string) error {
	args := m.Called(subject, message)
	return args.Error(0)
}

func TestNewTelnyxBalanceCheckTask(t *testing.T) {
	apiURL := "https://api.telnyx.com/v2/balance"
	apiKey := "KEY123"
	threshold := 10.0
	cooldown := 6 * time.Hour
	notifier := &MockNotifier{}

	task := NewTelnyxBalanceCheckTask(apiURL, apiKey, threshold, cooldown, notifier)

	assert.NotNil(t, task)
	assert.Equal(t, threshold, task.threshold)
	assert.Equal(t, cooldown, task.notificationCooldown)
	assert.NotNil(t, task.apiClient)
	assert.NotNil(t, task.notifier)
	assert.True(t, task.lastNotificationTime.IsZero())
}

func TestTelnyxBalanceCheckTask_Run_BalanceAboveThreshold(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(25.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	// Notifier should not be called when balance is above threshold
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
}

func TestTelnyxBalanceCheckTask_Run_BalanceBelowThreshold_SendsNotification(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(5.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", "Telnyx Balance Alert", mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "$5.00") && assert.Contains(t, msg, "$10.00")
	})).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
	assert.False(t, task.lastNotificationTime.IsZero())
}

func TestTelnyxBalanceCheckTask_Run_BalanceBelowThreshold_RespectsCooldown(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 1 * time.Hour,
		lastNotificationTime: time.Now().Add(-30 * time.Minute), // 30 minutes ago
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(5.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	// Should not send notification due to cooldown
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
}

func TestTelnyxBalanceCheckTask_Run_BalanceBelowThreshold_CooldownExpired(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 1 * time.Hour,
		lastNotificationTime: time.Now().Add(-2 * time.Hour), // 2 hours ago
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(5.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", "Telnyx Balance Alert", mock.Anything).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestTelnyxBalanceCheckTask_Run_APIError(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(0.0, errors.New("API connection failed"))
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get balance")
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
}

func TestTelnyxBalanceCheckTask_Run_NotificationError(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(5.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", "Telnyx Balance Alert", mock.Anything).Return(errors.New("notification failed"))
	task.notifier = mockNotifier

	err := task.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send notification")
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
	// lastNotificationTime should not be updated on error
	assert.True(t, task.lastNotificationTime.IsZero())
}

func TestTelnyxBalanceCheckTask_Run_BalanceExactlyAtThreshold(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(10.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	// Balance exactly at threshold should not trigger notification
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
}

func TestTelnyxBalanceCheckTask_Run_VeryLowBalance(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(0.01, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", "Telnyx Balance Alert", mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "$0.01")
	})).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestTelnyxBalanceCheckTask_Run_NegativeBalance(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(-5.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", "Telnyx Balance Alert", mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "$-5.00")
	})).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestTelnyxBalanceCheckTask_Run_MultipleCalls_UpdatesLastNotificationTime(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 1 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(5.0, nil).Times(2)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", "Telnyx Balance Alert", mock.Anything).Return(nil).Once()
	task.notifier = mockNotifier

	// First call - should send notification
	err := task.Run()
	require.NoError(t, err)
	firstNotificationTime := task.lastNotificationTime

	// Wait a bit but not past cooldown
	time.Sleep(10 * time.Millisecond)

	// Second call - should not send notification due to cooldown
	err = task.Run()
	require.NoError(t, err)

	// lastNotificationTime should be unchanged
	assert.Equal(t, firstNotificationTime, task.lastNotificationTime)

	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestTelnyxBalanceCheckTask_Run_ZeroThreshold(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            0.0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(5.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	// Positive balance above zero threshold
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything)
}

func TestTelnyxBalanceCheckTask_Run_FirstNotification(t *testing.T) {
	task := &TelnyxBalanceCheckTask{
		threshold:            10.0,
		notificationCooldown: 6 * time.Hour,
		lastNotificationTime: time.Time{}, // Zero time (never notified)
	}

	mockAPI := &MockTelnyxClient{}
	mockAPI.On("GetBalance").Return(5.0, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", "Telnyx Balance Alert", mock.Anything).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	// First notification should always go through regardless of cooldown
	mockNotifier.AssertExpectations(t)
	assert.False(t, task.lastNotificationTime.IsZero())
}