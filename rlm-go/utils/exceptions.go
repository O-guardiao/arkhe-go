package utils

import "fmt"

type BudgetExceededError struct {
	Spent  float64
	Budget float64
	Msg    string
}

func (e BudgetExceededError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("budget exceeded: spent $%.6f of $%.6f budget", e.Spent, e.Budget)
}

type TimeoutExceededError struct {
	Elapsed       float64
	Timeout       float64
	PartialAnswer string
	Msg           string
}

func (e TimeoutExceededError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("timeout exceeded: %.1fs of %.1fs limit", e.Elapsed, e.Timeout)
}

type TokenLimitExceededError struct {
	TokensUsed    int
	TokenLimit    int
	PartialAnswer string
	Msg           string
}

func (e TokenLimitExceededError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("token limit exceeded: %d of %d tokens", e.TokensUsed, e.TokenLimit)
}

type ErrorThresholdExceededError struct {
	ErrorCount    int
	Threshold     int
	LastError     string
	PartialAnswer string
	Msg           string
}

func (e ErrorThresholdExceededError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf(
		"error threshold exceeded: %d consecutive errors (limit: %d)",
		e.ErrorCount,
		e.Threshold,
	)
}

type CancellationError struct {
	PartialAnswer string
	Msg           string
}

func (e CancellationError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return "execution cancelled by user"
}
