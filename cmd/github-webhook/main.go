package main

import (
	"context"
	"net/http"
	"os"
	"sync"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	"github.com/michaelbeaumont/properator/pkg/githubwebhook"
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

	secret, err := githubwebhook.GetSecret("WEBHOOK_SECRET")
	if err != nil {
		log.Error(err, "couldn't get webhook secret")
		os.Exit(1)
	}

	setup, err := githubwebhook.SetupGhCli(context.Background())
	if err != nil {
		log.Error(err, "failed to setup gh clients")
		os.Exit(1)
	}

	events := make(chan interface{}, 200)
	worker := githubwebhook.NewWebhookWorker(k8s, setup.CliForInstall, setup.Username, log)

	var wg sync.WaitGroup

	wg.Add(1)

	go worker.Worker(&wg, events)

	wh := githubwebhook.NewWebhook(secret, events)
	Handler := http.NewServeMux()
	Handler.Handle("/webhook", &wh)

	s := &http.Server{
		Addr:    ":8080",
		Handler: Handler,
	}

	log.Info("Listening", "port", "8080", "username", setup.Username)
	log.Error(s.ListenAndServe(), "Failed to serve")
	close(events)
	wg.Wait()
	os.Exit(1)
}
