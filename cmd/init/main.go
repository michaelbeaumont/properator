package main

import (
	"bufio"
	"context"
	"io/ioutil"
	"log"
	"os"

	"github.com/pkg/browser"
	"github.com/pkg/errors"
)

const (
	envFile = ".env"
	keyFile = "id_rsa"
)

func waitForInput() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return scanner.Err()
}

func main() {
	ctx := context.Background()

	log.Println("Press Enter to begin app manifest flow and open browser.")

	err := waitForInput()
	if err != nil {
		log.Fatal(errors.Wrapf(err, "couldn't get user input"))
	}

	url, codeOrErrorRecv, err := StartFlow(ctx)
	if err != nil {
		log.Fatal(errors.Wrapf(err, "couldn't start app manifest flow"))
	}

	log.Printf("Waiting for user to create app in browser (%s).\n", url)

	err = browser.OpenURL(url)
	if err != nil {
		log.Fatal(errors.Wrapf(err, "Couldn't start app manifest flow"))
	}
	codeOrError := <-codeOrErrorRecv
	if codeOrError.Err != nil {
		log.Fatal(errors.Wrapf(codeOrError.Err, "Couldn't create app and receive code"))
	}
	log.Println("Received code from GitHub.")

	conversion, err := Exchange(codeOrError.Code)
	if err != nil {
		log.Fatal(errors.Wrapf(err, "Couldn't get app information"))
	}

	env, key := conversion.Output()

	err = ioutil.WriteFile(envFile, env, 0644)
	if err != nil {
		log.Fatal(errors.Wrapf(err, "Couldn't write env to %s", envFile))
	}
	log.Printf("Wrote env configuration to %s\n", envFile)

	err = ioutil.WriteFile(keyFile, key, 0644)
	if err != nil {
		log.Fatal(errors.Wrapf(err, "Couldn't write key to %s", keyFile))
	}
	log.Printf("Wrote key to %s\n", keyFile)
}
