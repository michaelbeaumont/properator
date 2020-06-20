package githubwebhook

import (
	"context"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/bradleyfalzon/ghinstallation"
	gh "github.com/google/go-github/v31/github"
	"github.com/pkg/errors"
)

const secretsPath = "/etc/secrets"

// GetSecret gets a secret from the mounted volume.
func GetSecret(k string) ([]byte, error) {
	contents, err := ioutil.ReadFile(filepath.Join(secretsPath, k))
	return contents, errors.Wrapf(err, "unable to get %s as mounted secret", k)
}

// ClientForInstallation creates a go-github client for a specific app
// installation.
type ClientForInstallation func(installationId int64) (*gh.Client, error)

// GhCliSetup is the return value for `SetupGhCli`.
type GhCliSetup struct {
	Username      string
	CliForInstall ClientForInstallation
	CliForApp     *gh.Client
}

// SetupGhCli abstracts away the github app aspect and gives us a username
// to pay attention to and a way to get a GH client.
func SetupGhCli(ctx context.Context) (GhCliSetup, error) {
	rawAppID, err := GetSecret("APP_ID")
	if err != nil {
		return GhCliSetup{}, err
	}

	appID, err := strconv.ParseInt(string(rawAppID), 10, 0)
	if err != nil {
		return GhCliSetup{}, errors.Wrap(err, "unable to parse APP_ID as int")
	}

	privateKey, err := GetSecret("id_rsa")
	if err != nil {
		return GhCliSetup{}, err
	}

	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, privateKey)
	if err != nil {
		return GhCliSetup{}, errors.Wrapf(err, "couldn't authenticate as app")
	}

	ghcli := gh.NewClient(&http.Client{Transport: transport})

	app, _, err := ghcli.Apps.Get(ctx, "")
	if err != nil {
		return GhCliSetup{}, errors.Wrapf(err, "problem getting username from github")
	}

	var makeGhCli ClientForInstallation = func(installationID int64) (*gh.Client, error) {
		transport, err := ghinstallation.New(http.DefaultTransport, appID, installationID, privateKey)
		if err != nil {
			return nil, errors.Wrapf(err, "couldn't create client for installation")
		}

		return gh.NewClient(&http.Client{Transport: transport}), nil
	}

	return GhCliSetup{app.GetSlug(), makeGhCli, ghcli}, nil
}
