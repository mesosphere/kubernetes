/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package framework

import (
	"errors"
	"io"
	"net/http"
	"time"

	log "github.com/golang/glog"
)

var (
	ActionTimeoutError = errors.New("timed out waiting for action to be processed")
	ActionAbortedError = errors.New("action aborted")
)

// action is the interface which executes
// a simple action inside a http transaction.
//
// execute takes an io.Writer (the http output) and returns an error.
type action interface {
	execute(w io.Writer) error
}

// actionFunc is an adapter to allow the use of ordinary functions as an Action.
type actionFunc func(w io.Writer) error

// execute calls f(w)
func (f actionFunc) execute(w io.Writer) error {
	return f(w)
}

// decorator is a function that takes an action and returns a decorated action.
type decorator func(action) action

// decorate takes an action and a list of decorators and returns
// a decorated action in the given order of decorators.
func decorate(a action, ds ...decorator) action {
	for _, decorate := range ds {
		a = decorate(a)
	}
	return a
}

// logger is a decorator that takes an action,
// logs a warning in case an error occured
// and returns the execution error.
func logger(a action) action {
	return actionFunc(func(w io.Writer) (err error) {
		if err = a.execute(w); err != nil {
			log.Warningf("error processing request: %v", err)
		}
		return
	})
}

// guard is a decorator that takes a timeout and an abort channel
// and executes the given action in a separate goroutine.
// If the action finishes first, its error is being returned.
//
// If the timeout or an event in the abort channel happens first
// an error is returned and the result of the executed action is ignored.
func guard(timeout time.Duration, abort <-chan struct{}) decorator {
	return func(a action) action {
		return actionFunc(func(w io.Writer) error {
			fin := make(chan error, 1)
			go func() {
				fin <- a.execute(w)
			}()

			var err error
			select {
			case err = <-fin:
				// action finished first
			case <-abort:
				err = ActionAbortedError
			case <-time.After(timeout):
				err = ActionTimeoutError
			}

			return err
		})
	}
}

// handler takes an action and returns a http Handler.
// It executes the action, passes the reponse writer as its argument.
// If the action returns an error, a 500 status code is returned, else 204 (no content).
func handler(a action) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := a.execute(w); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
