# Testing Guide for Watchdog

## Quick Start

### Run All Tests
```bash
go test ./...
```

### Run Tests with Verbose Output
```bash
go test ./... -v
```

### Run Tests with Coverage
```bash
go test ./... -cover
```

### Run Specific Package Tests
```bash
# API tests
go test ./internal/api/... -v

# Config tests
go test ./internal/config/... -v

# Scheduler tests
go test ./internal/scheduler/... -v

# Notifier tests
go test ./internal/notifier/... -v

# Task tests
go test ./tasks/... -v
```

## Test Suite Overview

This project now has **124 comprehensive unit tests** across **7 test files**, providing extensive coverage of all functionality introduced in this branch.

### Package Breakdown

| Package | Test File | Tests | Coverage Focus |
|---------|-----------|-------|----------------|
| `internal/api` | `github_test.go` | 16 | GitHub API client, PR fetching |
| `internal/api` | `telnyx_test.go` | 16 | Telnyx API client, balance checking |
| `internal/config` | `config_test.go` | 10 | Configuration parsing, validation |
| `internal/scheduler` | `scheduler_test.go` | 15 | Task scheduling, execution |
| `internal/notifier` | `webhook_test.go` | 15 | Webhook notifications via Apprise |
| `tasks` | `pr_review_check_test.go` | 29 | GitHub PR monitoring logic |
| `tasks` | `telnyx_balance_check_test.go` | 23 | Telnyx balance monitoring logic |

## Testing Philosophy

### 1. Comprehensive Coverage
Every public function and method has corresponding tests covering:
- ✅ Happy path scenarios
- ✅ Edge cases and boundary conditions
- ✅ Error handling and graceful degradation
- ✅ Invalid inputs and malformed data

### 2. Isolation
Tests are isolated and don't depend on:
- ❌ External services (APIs are mocked)
- ❌ Network connectivity
- ❌ Filesystem state
- ❌ Other tests

### 3. Fast Execution
- Tests run in milliseconds
- No real HTTP calls
- No database connections
- Suitable for CI/CD pipelines

### 4. Maintainability
- Clear, descriptive test names
- Table-driven tests for similar scenarios
- Well-structured test code
- Comprehensive comments

## Key Test Scenarios

### API Testing
```go
// Tests cover:
- Successful API responses
- HTTP error codes (401, 403, 404, 500, 503)
- Network timeouts
- Invalid JSON responses
- Authentication header validation
- Request parameter validation
```

### Configuration Testing
```go
// Tests cover:
- Duration parsing (5m, 1h, 24h, 1h30m45s)
- Default value fallbacks
- Invalid/empty input handling
- Comma-separated list parsing
- Edge cases (whitespace, trailing commas)
```

### Scheduler Testing
```go
// Tests cover:
- Task scheduling and execution
- Multiple concurrent tasks
- Correct interval timing
- Error handling (tasks continue on failure)
- Start/stop functionality
- Long-running task handling
```

### Task Testing
```go
// Tests cover:
- Stale PR detection
- Author filtering
- Notification cooldown
- Balance threshold checking
- API error handling
- Notification error handling
- Cleanup of old data
```

## Mocking Strategy

### Interfaces for Dependency Injection
We created interfaces to enable easy mocking:
- `api.GitHubClient` - For GitHub API operations
- `api.TelnyxClient` - For Telnyx API operations
- `notifier.Notifier` - For notification operations

### HTTP Mocking
```go
// Example: Mocking GitHub API
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Verify request
    assert.Equal(t, "GET", r.Method)
    
    // Send mock response
    json.NewEncoder(w).Encode(mockData)
}))
defer server.Close()
```

### Mock Objects with Testify
```go
// Example: Mocking notifier
mockNotifier := &MockNotifier{}
mockNotifier.On("SendNotification", "subject", "message").Return(nil)

// Use in test
task := NewTask(cfg, mockNotifier)
err := task.Run()

// Verify mock was called
mockNotifier.AssertExpectations(t)
```

## Common Test Patterns

### Table-Driven Tests
```go
func TestConfigParsing(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected time.Duration
    }{
        {"5 minutes", "5m", 5 * time.Minute},
        {"1 hour", "1h", 1 * time.Hour},
        {"invalid", "bad", defaultDuration},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := ParseDuration(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### Error Testing
```go
func TestAPIError(t *testing.T) {
    // Setup mock that returns error
    mockAPI := &MockAPI{}
    mockAPI.On("GetData").Return(nil, errors.New("API error"))
    
    // Execute
    result, err := task.Run()
    
    // Verify error is handled gracefully
    assert.Error(t, err)
    assert.Nil(t, result)
    assert.Contains(t, err.Error(), "API error")
}
```

### Concurrent Testing
```go
func TestConcurrency(t *testing.T) {
    scheduler := NewScheduler()
    task1 := &MockTask{}
    task2 := &MockTask{}
    
    scheduler.ScheduleTask(task1, 50*time.Millisecond)
    scheduler.ScheduleTask(task2, 50*time.Millisecond)
    scheduler.Start()
    
    time.Sleep(200 * time.Millisecond)
    scheduler.Stop()
    
    // Verify both tasks ran multiple times
    assert.Greater(t, task1.GetRunCount(), 0)
    assert.Greater(t, task2.GetRunCount(), 0)
}
```

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.25'
      - run: go test ./... -v -cover
```

## Troubleshooting

### Tests Fail Due to Timing
Some scheduler tests use short intervals (50ms). If they fail:
```bash
# Run with increased timeouts
go test ./internal/scheduler/... -timeout 30s
```

### Coverage Report
```bash
# Generate HTML coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Verbose Test Output
```bash
# See detailed test execution
go test ./... -v | tee test-output.txt
```

## Best Practices for Adding Tests

### 1. Test File Naming
- Test files: `*_test.go`
- Place in same package as code being tested
- One test file per source file

### 2. Test Function Naming
```go
// Pattern: Test<FunctionName>_<Scenario>
func TestGetBalance_Success(t *testing.T) { }
func TestGetBalance_APIError(t *testing.T) { }
func TestGetBalance_InvalidResponse(t *testing.T) { }
```

### 3. Use Table-Driven Tests
- Group similar test cases
- Easier to add new scenarios
- Better test coverage

### 4. Mock External Dependencies
- Never call real APIs in tests
- Use interfaces for dependency injection
- Use `httptest` for HTTP mocking

### 5. Assert Clearly
```go
// Good: Clear assertion with helpful message
assert.Equal(t, expected, actual, "balance should match")

// Better: Use require for fatal assertions
require.NoError(t, err, "API call should succeed")
```

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
- [Table Driven Tests](https://go.dev/wiki/TableDrivenTests)

## Summary

This test suite provides:
- ✅ **124 comprehensive tests** covering all functionality
- ✅ **Fast execution** (< 3 seconds for full suite)
- ✅ **No external dependencies** (all mocked)
- ✅ **High confidence** in code correctness
- ✅ **Easy maintenance** with clear patterns

All tests pass ✓ and are ready for CI/CD integration.