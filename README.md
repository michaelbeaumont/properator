# properator

properator manages launching _in progress_ versions of your application using Flux,
pull requests and the Github deployments API.

## Usage

Assuming `properator` is setup to listen to your repository's webhook events
(see below) and running as `@properator-bot`.
If we comment `@properator-bot deploy` on an existing PR, `properator` will launch
an instance of Flux
pointed to that PR's branch and create a GH deployment to track it.

<img src="docs/usage.png" width="600" alt="Usage">

When the PR is closed, that instance of Flux and the launched manifests will disappear.

<img src="docs/closed.png" width="600" alt="Drop">

Note: As more commits are pushed, github will say the deployment is "outdated".
This is a drawback of the deployments API; it doesn't let us update a
deployment, we can only create new ones.
Howeber, the deployment is not really forever out of date, Flux will apply the changes within 5
minutes.

### URL annotations

Include annotations like the following on an `Ingress` resource:

```
metadata:
  annotations:
    deploy.properator.io/deployment: github-webhook # This should always be `github-webhook`
    deploy.properator.io/url: https://2.pr.app.test # This should point to your deployment
```

to have the GH deployment point to `https://2.pr.app.test`.

#### Generation

Note: `properator` gives you access to the PR number
when manifests are generated on the file system at `/etc/properator`.

As a primitive example, this means you can have a `.flux.yaml` and `Ingress` like:

```
.flux.yaml
---
version: 1
patchUpdated:
  generators:
  - command: sed -e "s/\${PR}/$(cat /etc/properator/pr)/g" ingress.yaml
```

```
ingress.yaml
---
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: my-app
  annotations:
    deploy.properator.io/deployment: github-webhook
    deploy.properator.io/url: http://${PR}.pr.app.test
```

## Setup

We'll cover initializing a Github App for `properator` and then launching it
locally in `minikube`.

Note: requires kubernetes 1.16.

### Initialization

`properator` is meant to be run as a GitHub app. To make setup easier, execute:

```
go run ./cmd/init
```

This will setup the app in your account (organizations not yet supported) and write
the configuration and key to `.env`/`id_rsa`, which are later used to deploy `properator`.

#### Webhook

`properator` needs to listen to github webhook events. Visit
[smee](https://smee.io/) to get a publicly accessible webhook URL.
Enter this URL when initializing the app as above.

### Launch

At the moment, the images needs to be built manually and they need to end up
accessible by the cluster. For example, using `eval $(minikube docker-env)`,
execute:

```
make docker-build
```

Install the manifests to the cluster with:

```
make deploy
```

For `minikube` and testing, you can use `make listen-webhook` to use `smee.io`
to proxy events from the URL you created earlier to your local machine.

#### Deploy key

Setup a deploy key in your cluster that will be shared by `flux` instances to access the repository.

```
kubectl create ns properator-system
kubectl create secret generic -n properator-system flux-git-deploy --from-file=deploykey
```

where `identity` is a file containing a private SSH key. The public half should
be added as a deploy key in Github for each repo setup with `properator`.

## TODO

1. Add configuration to repositories
   - `--git-path` for `flux`
1. How to measure "successful" deployment?
   Right now it's just whether an `Ingress` resource appears with a link to the
   deployment.
   - If we're not using `Ingress`?
   - Check responsiveness of ingress/service and set the deployment when it's
     ready
1. Automate deploy key setup for flux
