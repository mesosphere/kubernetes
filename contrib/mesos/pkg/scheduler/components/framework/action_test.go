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
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

var actionErr = errors.New("some error!")

func TestGuard(t *testing.T) {
	for i, tt := range []struct {
		a       action
		timeout time.Duration
		abort   <-chan struct{}
		want    error
	}{
		{
			a:       actionFunc(func(io.Writer) error { return actionErr }),
			timeout: 100 * time.Millisecond,
			want:    actionErr,
		},
		{
			a: actionFunc(func(io.Writer) error {
				time.Sleep(200 * time.Millisecond)
				return nil
			}),
			timeout: 100 * time.Millisecond,
			want:    ActionTimeoutError,
		},
		{
			a: actionFunc(func(io.Writer) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			}),
			timeout: 100 * time.Millisecond,
			abort:   aborted(),
			want:    ActionAbortedError,
		},
	} {
		a := decorate(tt.a, guard(tt.timeout, tt.abort))
		var buf bytes.Buffer
		if got := a.execute(&buf); got != tt.want {
			t.Errorf("test %d got %v want %v", i, got, tt.want)
		}
	}
}

// aborted returns a closed (thus aborted) channel
func aborted() <-chan struct{} {
	aborted := make(chan struct{})
	close(aborted)
	return aborted
}
