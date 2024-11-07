package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvWithDefault(t *testing.T) {
	// Test with unset environment variable
	result := getEnvWithDefault("TEST_ENV_VAR", "default")
	assert.Equal(t, "default", result)

	// Test with set environment variable
	os.Setenv("TEST_ENV_VAR", "custom")
	result = getEnvWithDefault("TEST_ENV_VAR", "default")
	assert.Equal(t, "custom", result)
	os.Unsetenv("TEST_ENV_VAR")
}

func TestGetEnvAsIntWithDefault(t *testing.T) {
	// Test with unset environment variable
	result := getEnvAsIntWithDefault("TEST_INT", 42)
	assert.Equal(t, 42, result)

	// Test with valid integer
	os.Setenv("TEST_INT", "100")
	result = getEnvAsIntWithDefault("TEST_INT", 42)
	assert.Equal(t, 100, result)

	// Test with invalid integer
	os.Setenv("TEST_INT", "invalid")
	result = getEnvAsIntWithDefault("TEST_INT", 42)
	assert.Equal(t, 42, result)
	os.Unsetenv("TEST_INT")
}

func TestGetEnvAsInt64WithDefault(t *testing.T) {
	// Test with unset environment variable
	result := getEnvAsInt64WithDefault("TEST_INT64", 42)
	assert.Equal(t, int64(42), result)

	// Test with valid integer
	os.Setenv("TEST_INT64", "100")
	result = getEnvAsInt64WithDefault("TEST_INT64", 42)
	assert.Equal(t, int64(100), result)

	// Test with invalid integer
	os.Setenv("TEST_INT64", "invalid")
	result = getEnvAsInt64WithDefault("TEST_INT64", 42)
	assert.Equal(t, int64(42), result)
	os.Unsetenv("TEST_INT64")
}

func TestGetEnvAsBoolWithDefault(t *testing.T) {
	// Test with unset environment variable
	result := getEnvAsBoolWithDefault("TEST_BOOL", true)
	assert.True(t, result)

	// Test with valid boolean strings
	testCases := map[string]bool{
		"true":  true,
		"True":  true,
		"1":     true,
		"false": false,
		"False": false,
		"0":     false,
	}

	for input, expected := range testCases {
		os.Setenv("TEST_BOOL", input)
		result = getEnvAsBoolWithDefault("TEST_BOOL", !expected)
		assert.Equal(t, expected, result)
	}

	// Test with invalid boolean
	os.Setenv("TEST_BOOL", "invalid")
	result = getEnvAsBoolWithDefault("TEST_BOOL", true)
	assert.True(t, result)
	os.Unsetenv("TEST_BOOL")
}

func TestGetEnvAsDurationWithDefault(t *testing.T) {
	defaultDuration := 5 * time.Minute

	// Test with unset environment variable
	result := getEnvAsDurationWithDefault("TEST_DURATION", defaultDuration)
	assert.Equal(t, defaultDuration, result)

	// Test with valid duration
	os.Setenv("TEST_DURATION", "10m")
	result = getEnvAsDurationWithDefault("TEST_DURATION", defaultDuration)
	assert.Equal(t, 10*time.Minute, result)

	// Test with invalid duration
	os.Setenv("TEST_DURATION", "invalid")
	result = getEnvAsDurationWithDefault("TEST_DURATION", defaultDuration)
	assert.Equal(t, defaultDuration, result)
	os.Unsetenv("TEST_DURATION")
}
