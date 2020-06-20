package githubwebhook

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"

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

// WebhookWorker pulls webhook events off the chan
// and gives them to a WebhookHandler
type WebhookWorker struct {
	k8s         client.Client
	makeHandler func(installationID int64) (*WebhookHandler, error)
	log         logr.Logger
}

// WebhookHandler handles a specific event
type WebhookHandler struct {
	k8s      client.Client
	ghCli    *gh.Client
	username string
	log      logr.Logger
}

// NewWebhookWorker creates the state needed for a worker
func NewWebhookWorker(k8s client.Client, makeGhcli ClientForInstallation, username string, log logr.Logger) WebhookWorker {
	makeHandler := func(installationID int64) (*WebhookHandler, error) {
		ghcli, err := makeGhcli(installationID)
		if err != nil {
			return nil, err
		}
		return &WebhookHandler{k8s, ghcli, username, log}, nil
	}
	return WebhookWorker{
		k8s,
		makeHandler,
		log,
	}
}

// HasInstallation covers all relevant webhook events
type HasInstallation interface {
	GetInstallation() *gh.Installation
}

// Worker handles github events from a channel
func (webhook *WebhookWorker) Worker(wg *sync.WaitGroup, events <-chan interface{}) {
	defer wg.Done()
	for event := range events {
		hasInstallation, ok := event.(HasInstallation)
		if !ok {
			webhook.log.Error(errors.New("couldn't understand webhook event, no installation present"), "")
			continue
		}
		installationID := hasInstallation.GetInstallation().GetID()
		handler, err := webhook.makeHandler(installationID)
		if err != nil {
			webhook.log.Error(err, "couldn't initialize handler for installation %v", installationID)
		}
		if action := handler.handleEvent(event); action != nil {
			if desc := action.Describe(); desc != "" {
				webhook.log.Info(desc)
			}
			if err := action.Act(handler); err != nil {
				webhook.log.Error(err, "Error doing an action")
			}
		}
	}
}

func (webhook *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, err := gh.ValidatePayload(r, webhook.webhookSecretKey)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := gh.ParseWebHook(gh.WebHookType(r), payload)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	select {
	case webhook.events <- event:
		return
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
}

func containsCommand(name, body, command string, otherCommands ...string) bool {
	commands := append([]string{command}, otherCommands...)
	pat := fmt.Sprintf("@%s[[:space:]]+(?:%s)", name, strings.Join(commands, "|"))
	matched, _ := regexp.MatchString(pat, body)
	return matched
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

	if containsCommand(username, body, "deploy") {
		return &create{
			owner: comment.GetRepo().GetOwner().GetLogin(),
			name:  comment.GetRepo().GetName(),
			pr:    pr,
		}
	}
	if containsCommand(username, body, "drop", "delete") {
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
func (webhook *WebhookHandler) handleEvent(event interface{}) action {
	switch event := event.(type) {
	case *gh.IssueCommentEvent:
		return parseComment(webhook.username, event)
	case *gh.PullRequestEvent:
		return parsePREvent(event)
	default:
		return nil
	}
}
