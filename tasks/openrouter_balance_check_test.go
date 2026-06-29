package tasks

import (
	"context"
	"errors"
	"testing"
	"time"
	"watchdog/internal/api"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockOpenRouterClient mocks the OpenRouter API client.
type MockOpenRouterClient struct {
	mock.Mock
}

func (m *MockOpenRouterClient) GetAuthKey(ctx context.Context) (*api.OpenRouterKeyResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.OpenRouterKeyResponse), args.Error(1)
}

func (m *MockOpenRouterClient) GetCredits(ctx context.Context) (*api.OpenRouterCreditsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.OpenRouterCreditsResponse), args.Error(1)
}

func (m *MockOpenRouterClient) GetActivity(ctx context.Context, date, apiKeyHash string) (*api.OpenRouterActivityResponse, error) {
	args := m.Called(ctx, date, apiKeyHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.OpenRouterActivityResponse), args.Error(1)
}

func limitPtr(v float64) *float64 {
	return &v
}

func TestNewOpenRouterBalanceCheckTask(t *testing.T) {
	baseURL := "https://openrouter.ai/api/v1"
	apiKey := "sk-or-v1-test"
	balanceThreshold := 10.0
	usageLimitRatio := 0.8
	cooldown := 6 * time.Hour
	mockNotifier := &MockNotifier{}

	task := NewOpenRouterBalanceCheckTask(baseURL, apiKey, balanceThreshold, usageLimitRatio, cooldown, mockNotifier)

	assert.NotNil(t, task)
	assert.Equal(t, balanceThreshold, task.balanceThreshold)
	assert.Equal(t, usageLimitRatio, task.usageLimitRatio)
	assert.Equal(t, cooldown, task.notificationCooldown)
	assert.NotNil(t, task.apiClient)
	assert.NotNil(t, task.notifier)
	assert.True(t, task.lastNotificationTime.IsZero())
}

func TestOpenRouterBalanceCheckTask_Run_NoAlerts(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      95.0,
			Limit:      limitPtr(500.0),
			IsFreeTier: true,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 100.0,
			TotalUsage:   25.0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestOpenRouterBalanceCheckTask_Run_BalanceBelowThreshold(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      25.0,
			Limit:      limitPtr(500.0),
			IsFreeTier: true,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 100.0,
			TotalUsage:   95.0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "OpenRouter Credits Alert", mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "$5.00") && assert.Contains(t, msg, "$10.00")
	})).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
	assert.False(t, task.lastNotificationTime.IsZero())
}

func TestOpenRouterBalanceCheckTask_Run_UsageExceedsLimitRatio(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      450.0,
			Limit:      limitPtr(500.0),
			IsFreeTier: true,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 500.0,
			TotalUsage:   450.0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "OpenRouter Credits Alert", mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "90%") && assert.Contains(t, msg, "80%")
	})).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestOpenRouterBalanceCheckTask_Run_BothConditions(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      450.0,
			Limit:      limitPtr(500.0),
			IsFreeTier: true,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 455.0,
			TotalUsage:   450.0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "OpenRouter Credits Alert", mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "5.00") && assert.Contains(t, msg, "90%")
	})).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestOpenRouterBalanceCheckTask_Run_NoLimitSet(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      0,
			Limit:      nil,
			IsFreeTier: false,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 100.0,
			TotalUsage:   0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestOpenRouterBalanceCheckTask_Run_RespectsCooldown(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 1 * time.Hour,
		lastNotificationTime: time.Now().Add(-30 * time.Minute),
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      25.0,
			Limit:      limitPtr(500.0),
			IsFreeTier: true,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 30.0,
			TotalUsage:   25.0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestOpenRouterBalanceCheckTask_Run_CooldownExpired(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 1 * time.Hour,
		lastNotificationTime: time.Now().Add(-2 * time.Hour),
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      25.0,
			Limit:      limitPtr(500.0),
			IsFreeTier: true,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 30.0,
			TotalUsage:   25.0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "OpenRouter Credits Alert", mock.Anything).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestOpenRouterBalanceCheckTask_Run_APIError(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(nil, errors.New("API connection failed"))
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get auth key info")
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestOpenRouterBalanceCheckTask_Run_NotificationError(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      25.0,
			Limit:      limitPtr(500.0),
			IsFreeTier: true,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 30.0,
			TotalUsage:   25.0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "OpenRouter Credits Alert", mock.Anything).Return(errors.New("notification failed"))
	task.notifier = mockNotifier

	err := task.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send notification")
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
	assert.True(t, task.lastNotificationTime.IsZero())
}

func TestOpenRouterBalanceCheckTask_Run_BalanceThresholdDisabled(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
	}

	limit := 500.0
	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      100.0,
			Limit:      &limit,
			IsFreeTier: true,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	// Balance is 0 but threshold is 0 (disabled), usage is 100/500 = 20% which is under 80%
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestOpenRouterBalanceCheckTask_Run_UsageLimitRatioDisabled(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0,
		notificationCooldown: 6 * time.Hour,
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 100.0,
			TotalUsage:   0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestOpenRouterBalanceCheckTask_Run_DailyFreeRequestsExceedsRatio(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		dailyRequestLimit:    300,
		dailyRequestRatio:    0.8,
		dailyRequestFreeOnly: true,
		notificationCooldown: 6 * time.Hour,
		activityAPIKeyHash:   "",
		now:                  func() time.Time { return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC) },
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetActivity", mock.Anything, "2026-06-03", "").Return(&api.OpenRouterActivityResponse{
		Data: []api.OpenRouterActivityEntry{
			{
				Usage:    0,
				Requests: 274,
			},
			{
				Usage:    0,
				Requests: 16,
			},
			{
				Usage:    0,
				Requests: 1,
			},
			{
				Usage:    0.000066,
				Requests: 1,
			},
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "OpenRouter Credits Alert", mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "free requests") &&
			assert.Contains(t, msg, "2026-06-03") &&
			assert.Contains(t, msg, "291 / 300")
	})).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
	assert.Equal(t, "2026-06-03", task.lastKnownDailyRequestDate)
	assert.Equal(t, 291, task.lastKnownDailyRequests)
	assert.False(t, task.lastNotificationTime.IsZero())
}

func TestOpenRouterBalanceCheckTask_Run_DailyRequestsBelowRatio(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		dailyRequestLimit:    1000,
		dailyRequestRatio:    0.8,
		dailyRequestFreeOnly: true,
		notificationCooldown: 6 * time.Hour,
		now:                  func() time.Time { return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC) },
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetActivity", mock.Anything, "2026-06-03", "").Return(&api.OpenRouterActivityResponse{
		Data: []api.OpenRouterActivityEntry{
			{
				Usage:    0,
				Requests: 799,
			},
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestOpenRouterBalanceCheckTask_Run_DailyRequestsActivityError(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		dailyRequestLimit:    1000,
		dailyRequestRatio:    0.8,
		dailyRequestFreeOnly: true,
		notificationCooldown: 6 * time.Hour,
		now:                  func() time.Time { return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC) },
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetActivity", mock.Anything, "2026-06-03", "").Return(nil, errors.New("activity failed"))
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	task.notifier = mockNotifier

	err := task.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get activity for 2026-06-03")
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestLatestCompletedUTCDate(t *testing.T) {
	got := latestCompletedUTCDate(time.Date(2026, 6, 4, 12, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60)))

	assert.Equal(t, "2026-06-03", got)
}

func TestCountOpenRouterActivityRequests(t *testing.T) {
	entries := []api.OpenRouterActivityEntry{
		{
			Usage:        0,
			Requests:     10,
			BYOKRequests: 2,
		},
		{
			Usage:    0.01,
			Requests: 20,
		},
		{
			Usage:              0,
			BYOKUsageInference: 0.5,
			Requests:           30,
		},
		{
			Usage:        0,
			Requests:     1,
			BYOKRequests: 3,
		},
	}

	assert.Equal(t, 8, countOpenRouterActivityRequests(entries, true))
	assert.Equal(t, 61, countOpenRouterActivityRequests(entries, false))
}

func TestOpenRouterBalanceCheckTask_Run_FirstNotification(t *testing.T) {
	task := &OpenRouterBalanceCheckTask{
		balanceThreshold:     10.0,
		usageLimitRatio:      0.8,
		notificationCooldown: 6 * time.Hour,
		lastNotificationTime: time.Time{},
	}

	mockAPI := &MockOpenRouterClient{}
	mockAPI.On("GetAuthKey", mock.Anything).Return(&api.OpenRouterKeyResponse{
		Data: struct {
			Usage      float64  `json:"usage"`
			Limit      *float64 `json:"limit"`
			IsFreeTier bool     `json:"is_free_tier"`
		}{
			Usage:      0,
			Limit:      nil,
			IsFreeTier: false,
		},
	}, nil)
	mockAPI.On("GetCredits", mock.Anything).Return(&api.OpenRouterCreditsResponse{
		Data: struct {
			TotalCredits float64 `json:"total_credits"`
			TotalUsage   float64 `json:"total_usage"`
		}{
			TotalCredits: 5.0,
			TotalUsage:   0,
		},
	}, nil)
	task.apiClient = mockAPI

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "OpenRouter Credits Alert", mock.Anything).Return(nil)
	task.notifier = mockNotifier

	err := task.Run()

	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
	assert.False(t, task.lastNotificationTime.IsZero())
}
