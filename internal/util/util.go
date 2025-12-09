package util

import (
	"fmt"
	"os"
	"strings"
)

// MaskEnvArg masks sensitive environment variable values for printing.
// Example: POSTGRES_PASSWORD=abc -> POSTGRES_PASSWORD=***
func MaskEnvArg(arg string) string {
	if strings.HasPrefix(arg, "PG_PASSWORD=") {
		return "PG_PASSWORD=***"
	}
	if strings.HasPrefix(arg, "PGPASSWORD=") {
		return "PG_PASSWORD=***"
	}

	return arg
}

// MaskArgs applies MaskEnvArg to an entire slice.
func MaskArgs(args []string) []string {
	masked := make([]string, len(args))
	for i, a := range args {
		masked[i] = MaskEnvArg(a)
	}
	return masked
}

// GetRequiredEnv returns the value of the given environment variable or an error if unset.
func GetRequiredEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("%s is not set (put it in .env)", key)
	}
	return value, nil
}
func FormatArgs(args []string) string {
    var result string
    for _, a := range args {
        if ContainsWhitespace(a) {
            result += fmt.Sprintf("'%s' ", a)
        } else {
            result += fmt.Sprintf("%s ", a)
        }
    }
    return result
}
func ContainsWhitespace(s string) bool {
    for _, r := range s {
        if r == ' ' || r == '\t' || r == '\n' {
            return true
        }
    }
    return false
}
