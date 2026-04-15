package rlm

import (
	"github.com/alexzhang13/rlm-go/core"
	"github.com/alexzhang13/rlm-go/logger"
	"github.com/alexzhang13/rlm-go/types"
	"github.com/alexzhang13/rlm-go/utils"
)

type RLM = core.RLM
type Config = core.Config

type RLMChatCompletion = types.RLMChatCompletion
type UsageSummary = types.UsageSummary
type ModelUsageSummary = types.ModelUsageSummary

type RLMLogger = logger.RLMLogger

type BudgetExceededError = utils.BudgetExceededError
type TimeoutExceededError = utils.TimeoutExceededError
type TokenLimitExceededError = utils.TokenLimitExceededError
type ErrorThresholdExceededError = utils.ErrorThresholdExceededError
type CancellationError = utils.CancellationError

func New(config Config) (*RLM, error) {
	return core.New(config)
}
