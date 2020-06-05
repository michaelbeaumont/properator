package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

// CodeOrError either contains a code or error
type CodeOrError struct {
	Code string
	Err  error
}

type listener struct {
	flow chan CodeOrError
}

const serverError = 500

func writePage(w http.ResponseWriter, page string) error {
	bytes, err := ioutil.ReadFile(fmt.Sprintf("./cmd/init/%s.html", page))
	if err != nil {
		w.WriteHeader(serverError)
		return errors.Errorf("Couldn't load HTML: %v", err)
	}

	w.Header().Set("Content-Type", "text/html")

	_, err = w.Write(bytes)
	if err != nil {
		w.WriteHeader(serverError)
		return errors.Errorf("Couldn't load HTML: %v", err)
	}

	return nil
}

func (l *listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "callback") {
		defer close(l.flow)

		q := r.URL.Query()
		code := q.Get("code")
		l.flow <- CodeOrError{code, nil}

		err := writePage(w, "finish")
		if err != nil {
			l.flow <- CodeOrError{Err: err}
		}
	} else {
		err := writePage(w, "create")
		if err != nil {
			l.flow <- CodeOrError{Err: err}
		}
	}
}

// StartFlow prompts the user to create a new app and listens for redirects.
func StartFlow(ctx context.Context) (url string, recv chan CodeOrError, err error) {
	flow := make(chan CodeOrError, 1)
	listenerFlow := make(chan CodeOrError, 1)
	s := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: &listener{listenerFlow},
	}

	sock, err := net.Listen("tcp4", s.Addr)
	if err != nil {
		return "", nil, err
	}

	port := sock.Addr().(*net.TCPAddr).Port

	go func() {
		if err := s.Serve(sock); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Error listening for response: %v", err)
			}
		}
	}()
	go func() {
		cs := <-listenerFlow

		if err := s.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
		flow <- cs
	}()

	return fmt.Sprintf("http://127.0.0.1:%d", port), flow, nil
}
