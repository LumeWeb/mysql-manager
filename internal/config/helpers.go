package config

import (
    "os"
    "strconv"
    "strings"
    "time"
)

func getEnvWithDefault(key string, defaultVal string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultVal
}

func getEnvAsInt64WithDefault(key string, defaultVal int64) int64 {
    if value := os.Getenv(key); value != "" {
        if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
            return intVal
        }
    }
    return defaultVal
}

func getEnvAsBoolWithDefault(key string, defaultVal bool) bool {
    if value := os.Getenv(key); value != "" {
        switch strings.ToLower(value) {
        case "true", "1", "yes", "on":
            return true
        case "false", "0", "no", "off":
            return false
        }
    }
    return defaultVal
}

func getEnvAsIntWithDefault(key string, defaultVal int) int {
    if value := os.Getenv(key); value != "" {
        if intVal, err := strconv.Atoi(value); err == nil {
            return intVal
        }
    }
    return defaultVal
}

func (c *Config) loadAPIConfig() {
    c.API.Port = getEnvAsIntWithDefault("API_PORT", 8080)
    c.API.Username = os.Getenv("API_USERNAME")
    c.API.Password = os.Getenv("API_PASSWORD")

    // If no username/password provided, generate a random one
    if c.API.Username == "" {
        c.API.Username = generateRandomUsername()
    }
    if c.API.Password == "" {
        c.API.Password = generateSecurePassword()
    }
}

func generateRandomUsername() string {
    // Implement random username generation
    return "admin-" + strconv.Itoa(int(time.Now().UnixNano()))
}

func generateSecurePassword() string {
    // Implement secure password generation
    return "secure-" + strconv.Itoa(int(time.Now().UnixNano()))
}

func getEnvAsDurationWithDefault(key string, defaultVal time.Duration) time.Duration {
    if value := os.Getenv(key); value != "" {
        if duration, err := time.ParseDuration(value); err == nil {
            return duration
        }
    }
    return defaultVal
}
