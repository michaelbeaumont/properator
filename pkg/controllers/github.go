package controllers

import (
	"context"

	gh "github.com/google/go-github/v31/github"
	"github.com/michaelbeaumont/properator/pkg/githubwebhook"
)

// ClientForOwnerRepo gives us a gh client for a specific installation.
type ClientForOwnerRepo func(ctx context.Context, owner, repo string) (*gh.Client, error)

// ClientForOwnerRepoFromSetup gives us a ClientForOwnerRepo functions.
func ClientForOwnerRepoFromSetup(ghCliSetup githubwebhook.GhCliSetup) ClientForOwnerRepo {
	return func(ctx context.Context, owner, repo string) (*gh.Client, error) {
		inst, _, err := ghCliSetup.CliForApp.Apps.FindRepositoryInstallation(ctx, owner, repo)
		if err != nil {
			return nil, err
		}

		return ghCliSetup.CliForInstall(inst.GetID())
	}
}
