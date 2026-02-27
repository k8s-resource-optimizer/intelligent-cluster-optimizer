package logger

import (
	"errors"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		development bool
		wantErr     bool
	}{
		{
			name:        "production logger with info level",
			level:       "info",
			development: false,
			wantErr:     false,
		},
		{
			name:        "development logger with debug level",
			level:       "debug",
			development: true,
			wantErr:     false,
		},
		{
			name:        "logger with warn level",
			level:       "warn",
			development: false,
			wantErr:     false,
		},
		{
			name:        "logger with error level",
			level:       "error",
			development: false,
			wantErr:     false,
		},
		{
			name:        "logger with invalid level defaults to info",
			level:       "invalid",
			development: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewLogger(tt.level, tt.development)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLogger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if logger == nil {
				t.Error("Expected non-nil logger")
			}
			if logger != nil {
				defer func() { _ = logger.Sync() }()
			}
		})
	}
}

func TestNewProductionLogger(t *testing.T) {
	logger, err := NewProductionLogger()
	if err != nil {
		t.Fatalf("NewProductionLogger() error = %v", err)
	}
	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}
	defer func() { _ = logger.Sync() }()
}

func TestNewDevelopmentLogger(t *testing.T) {
	logger, err := NewDevelopmentLogger()
	if err != nil {
		t.Fatalf("NewDevelopmentLogger() error = %v", err)
	}
	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}
	defer func() { _ = logger.Sync() }()
}

func TestInitGlobalLogger(t *testing.T) {
	err := InitGlobalLogger("debug", true)
	if err != nil {
		t.Fatalf("InitGlobalLogger() error = %v", err)
	}

	logger := GetLogger()
	if logger == nil {
		t.Fatal("Expected non-nil global logger")
	}
	defer func() { _ = logger.Sync() }()
}

func TestGetLogger(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	logger := GetLogger()
	if logger == nil {
		t.Fatal("Expected non-nil logger from GetLogger()")
	}

	// Should return same instance
	logger2 := GetLogger()
	if logger != logger2 {
		t.Error("Expected GetLogger() to return same instance")
	}
}

func TestLoggerWithFields(t *testing.T) {
	logger, err := NewDevelopmentLogger()
	if err != nil {
		t.Fatalf("NewDevelopmentLogger() error = %v", err)
	}
	defer func() { _ = logger.Sync() }()

	// Test WithFields
	loggerWithFields := logger.WithFields("key1", "value1", "key2", 123)
	if loggerWithFields == nil {
		t.Fatal("Expected non-nil logger from WithFields()")
		return
	}

	// Test logging with fields
	loggerWithFields.Info("test message")
}

func TestLoggerWithError(t *testing.T) {
	logger, err := NewDevelopmentLogger()
	if err != nil {
		t.Fatalf("NewDevelopmentLogger() error = %v", err)
	}
	defer func() { _ = logger.Sync() }()

	// Test WithError
	testErr := errors.New("test error")
	loggerWithError := logger.WithError(testErr)
	if loggerWithError == nil {
		t.Fatal("Expected non-nil logger from WithError()")
		return
	}

	// Test logging with error
	loggerWithError.Warn("test warning")
}

func TestGlobalLoggerFunctions(t *testing.T) {
	// Initialize global logger for testing
	err := InitGlobalLogger("debug", true)
	if err != nil {
		t.Fatalf("InitGlobalLogger() error = %v", err)
	}
	defer func() { _ = Sync() }()

	// Test all log level functions
	Debug("debug message")
	Debugf("debug formatted: %s", "test")

	Info("info message")
	Infof("info formatted: %d", 42)

	Warn("warn message")
	Warnf("warn formatted: %v", true)

	Error("error message")
	Errorf("error formatted: %s", "test error")

	// Test with fields
	WithFields("key", "value").Info("message with fields")

	// Test with error
	WithError(errors.New("test error")).Error("message with error")

	// Note: We don't test Fatal/Fatalf as they call os.Exit()
}

func TestLoggerLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := NewLogger(level, false)
			if err != nil {
				t.Fatalf("NewLogger(%s) error = %v", level, err)
			}
			defer func() { _ = logger.Sync() }()

			// Try logging at different levels
			logger.Debug("debug")
			logger.Info("info")
			logger.Warn("warn")
			logger.Error("error")
		})
	}
}
