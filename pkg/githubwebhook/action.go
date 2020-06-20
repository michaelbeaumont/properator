package githubwebhook

import (
	"fmt"
)

type action interface {
	Act(webhook *WebhookHandler) error
	Describe() string
}

type prPointer struct {
	id     int64
	number int
}

func (pr prPointer) getNamespaced() (string, string) {
	return "github-webhook", fmt.Sprintf("properator-github-webhook-%v-%v", pr.id, pr.number)
}

type noopAction struct {
}

func (ca *noopAction) Act(webhook *WebhookHandler) error {
	return nil
}

func (ca *noopAction) Describe() string {
	return ""
}

var (
	transientEnvironment = true
	readOnlyKey          = true
	autoMerge            = false
	properator           = "properator"
	success              = "success"
	inactive             = "inactive"
)

const annotation = "deploy.properator.io/github-webhook"
