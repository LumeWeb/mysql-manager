package config

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func ParseCronSchedule(schedule string) (bool, error) {
	if schedule == "" {
		return false, fmt.Errorf("empty schedule")
	}

	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return false, fmt.Errorf("invalid number of fields in schedule")
	}

	// Basic validation of cron fields
	for i, field := range fields {
		// Check each character in the field
		for _, c := range field {
			if !unicode.IsDigit(c) && c != ',' && c != '-' && c != '/' && c != '*' {
				return false, fmt.Errorf("invalid character in field: %c", c)
			}
		}

		// Handle single numbers
		if containsOnlyDigits(field) {
			val, err := strconv.Atoi(field)
			if err != nil {
				return false, fmt.Errorf("invalid field value: %s", field)
			}

			// Validate ranges based on field position
			switch i {
			case 0: // Minutes
				if val < 0 || val > 59 {
					return false, fmt.Errorf("minute value must be between 0-59")
				}
			case 1: // Hours
				if val < 0 || val > 23 {
					return false, fmt.Errorf("hour value must be between 0-23")
				}
			case 2: // Day of month
				if val < 1 || val > 31 {
					return false, fmt.Errorf("day value must be between 1-31")
				}
			case 3: // Month
				if val < 1 || val > 12 {
					return false, fmt.Errorf("month value must be between 1-12")
				}
			case 4: // Day of week
				if val < 0 || val > 6 {
					return false, fmt.Errorf("day of week value must be between 0-6")
				}
			}
		}
	}

	return true, nil
}

// Helper function to check if string contains only digits
func containsOnlyDigits(s string) bool {
	for _, c := range s {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}
