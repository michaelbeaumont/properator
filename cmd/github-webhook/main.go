package main

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/google/go-github/v31/github"
	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	gh "github.com/michaelbeaumont/properator/pkg/github"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func getClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = deployv1alpha1.AddToScheme(scheme)

	config := ctrl.GetConfigOrDie()

	return client.New(config, client.Options{
		Scheme: scheme,
	})
}

func main() {
	log := ctrl.Log.WithName("webhook")
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	k8s, err := getClient()

	if err != nil {
		log.Error(err, "problem creating client")
		os.Exit(1)
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Error(err, "couldn't find GITHUB_TOKEN in environment")
		os.Exit(1)
	}

	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if secret == "" {
		log.Error(err, "couldn't find GITHUB_WEBHOOK_SECRET in environment")
		os.Exit(1)
	}

	var wg sync.WaitGroup

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	ctx := context.Background()
	tc := oauth2.NewClient(ctx, ts)
	ghcli := github.NewClient(tc)
	user, _, err := ghcli.Users.Get(ctx, "")

	if err != nil {
		log.Error(err, "problem authenticating to github")
	}

	events := make(chan interface{}, 200)
	worker := gh.NewWebhookWorker(k8s, ghcli, user.GetLogin(), log)

	wg.Add(1)

	go worker.Worker(&wg, events)

	wh := gh.NewWebhook([]byte(secret), events)
	Handler := http.NewServeMux()
	Handler.Handle("/webhook", &wh)

	s := &http.Server{
		Addr:    ":8080",
		Handler: Handler,
	}

	log.Info("Listening", "port", "8080", "username", user.GetLogin())
	log.Error(s.ListenAndServe(), "Failed to serve")
	close(events)
	wg.Wait()
	os.Exit(1)
}
