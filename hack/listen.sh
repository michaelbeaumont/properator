#!/usr/bin/env bash
onINT() {
	echo "Killing port-forward $command1PID too"
	kill -INT "$command1PID"
	exit
}
: ${WEBHOOK_URL?Please set WEBHOOK_URL to a smee proxy URL (see https://smee.io/)}

trap "onINT" SIGINT
kubectl port-forward -n properator-system svc/properator-github-webhook 8080:443 &
command1PID="$!"
npx -p smee-client smee -p 8080 -u $WEBHOOK_URL -t http://localhost:8080/webhook
echo Done
