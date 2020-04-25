package github

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	gh "github.com/google/go-github/v31/github"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=deploy.properator.io,resources=refreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete

// Webhook is the state we need to handle webhook events
type Webhook struct {
	webhookSecretKey []byte
	events           chan interface{}
}

// NewWebhook creates the state needed for a webhook
func NewWebhook(key []byte, events chan interface{}) Webhook {
	return Webhook{
		key,
		events,
	}
}

// WebhookWorker handles webhook events
type WebhookWorker struct {
	k8s      client.Client
	ghcli    *gh.Client
	username string
	log      logr.Logger
}

// NewWebhookWorker creates the state needed for a worker
func NewWebhookWorker(k8s client.Client, ghcli *gh.Client, username string, log logr.Logger) WebhookWorker {
	return WebhookWorker{
		k8s,
		ghcli,
		username,
		log,
	}
}

// Worker handles github events from a channel
func (webhook *WebhookWorker) Worker(wg *sync.WaitGroup, events <-chan interface{}) {
	defer wg.Done()
	for event := range events {
		if action := webhook.handleEvent(event); action != nil {
			if desc := action.Describe(); desc != "" {
				webhook.log.Info(desc)
			}
			if err := action.Act(*webhook); err != nil {
				webhook.log.Error(err, "Error doing an action")
			}
		}
	}
}

func (webhook *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, err := gh.ValidatePayload(r, webhook.webhookSecretKey)

	if err != nil {
		http.Error(w, "", 400)
		return
	}

	event, err := gh.ParseWebHook(gh.WebHookType(r), payload)
	if err != nil {
		http.Error(w, "", 400)
		return
	}

	select {
	case webhook.events <- event:
		return
	default:
		http.Error(w, "", 503)
		return
	}
}

func parseComment(username string, comment *gh.IssueCommentEvent) action {
	if !comment.Issue.IsPullRequest() {
		return nil
	}
	pr := prPointer{
		number: comment.GetIssue().GetNumber(),
		id:     comment.GetRepo().GetID(),
	}
	body := comment.Comment.GetBody()

	if strings.Contains(body, fmt.Sprintf("@%s deploy", username)) {
		return &create{
			owner: comment.GetRepo().GetOwner().GetLogin(),
			name:  comment.GetRepo().GetName(),
			pr:    pr,
		}
	}
	if strings.Contains(body, fmt.Sprintf("@%s drop", username)) {
		return &drop{
			pr: pr,
		}
	}
	return &noopAction{}
}

func parsePREvent(event *gh.PullRequestEvent) action {
	pr := prPointer{
		number: event.GetPullRequest().GetNumber(),
		id:     event.GetRepo().GetID(),
	}
	switch *event.Action {
	case "edited":
		return &create{
			owner: event.GetRepo().GetOwner().GetLogin(),
			name:  event.GetRepo().GetName(),
			pr:    pr,
		}
	case "closed":
		return &drop{
			pr: pr,
		}
	default:
		return nil
	}
}

func (webhook *WebhookWorker) handleEvent(event interface{}) action {
	switch event := event.(type) {
	case *gh.IssueCommentEvent:
		return parseComment(webhook.username, event)
	case *gh.PullRequestEvent:
		return parsePREvent(event)
	default:
		return nil
	}
}
