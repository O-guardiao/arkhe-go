package rlm

import (
	"github.com/O-guardiao/arkhe-go/rlm-go/core"
	"github.com/O-guardiao/arkhe-go/rlm-go/logger"
	"github.com/O-guardiao/arkhe-go/rlm-go/types"
	"github.com/O-guardiao/arkhe-go/rlm-go/utils"
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
