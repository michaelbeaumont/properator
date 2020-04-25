package github

import (
	"testing"

	"github.com/google/go-github/v31/github"
	"github.com/stretchr/testify/assert"
)

var (
	owner = "michaelbeaumont"
	name  = "test"
)

func TestParseComment(t *testing.T) {
	num := 23
	id := int64(12345)
	body := "@properator deploy"
	commentEvent := github.IssueCommentEvent{
		Issue: &github.Issue{
			Number: &num,
		},
		Comment: &github.IssueComment{
			Body: &body,
		},
		Repo: &github.Repository{
			ID: &id,
			Owner: &github.User{
				Login: &owner,
			},
			Name: &name,
		},
	}
	parsed := parseComment("properator", &commentEvent)
	assert.Nil(t, parsed, "issue should be ignored without pull_request")
	commentEvent.Issue.PullRequestLinks = &github.PullRequestLinks{}
	parsed = parseComment("properator", &commentEvent)
	action := &create{owner: owner, name: name, pr: prPointer{number: num, id: id}}
	assert.Equal(t, action, parsed)
}
