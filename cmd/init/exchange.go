package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Conversion holds the interesting parts of the conversion response.
type Conversion struct {
	ID            int    `json:"id"`
	PEM           string `json:"pem"`
	WebhookSecret string `json:"webhook_secret"`
}

// asEnv outputs a key and value for an .env file.
func asEnv(key, value string) string {
	return fmt.Sprintf("%s=%s\n", key, value)
}

// ToEnv creates an env file from Conversion info.
func (c *Conversion) Output() ([]byte, []byte) {
	b := strings.Builder{}
	b.WriteString(asEnv("APP_ID", strconv.Itoa(c.ID)))
	b.WriteString(asEnv("WEBHOOK_SECRET", c.WebhookSecret))

	return []byte(b.String()), []byte(c.PEM)
}

// Exchange completes the app manifest flow
func Exchange(code string) (*Conversion, error) {
	conversionResp, err := http.Post(
		fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code),
		"application/json",
		bytes.NewBufferString(""),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't convert code")
	}
	defer conversionResp.Body.Close()

	body, err := ioutil.ReadAll(conversionResp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't read code conversion response")
	}

	conversion := Conversion{}
	err = json.Unmarshal(body, &conversion)

	if err != nil {
		return nil, errors.Wrapf(err, "error parsing returned app info")
	}

	return &conversion, nil
}
