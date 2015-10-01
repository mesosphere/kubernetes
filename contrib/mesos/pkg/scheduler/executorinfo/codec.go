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

package executorinfo

import (
	"bytes"
	"encoding/base64"
	"io"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/mesosproto"
)

var base64Codec = base64.StdEncoding

func EncodeResources(w io.Writer, rs []*mesosproto.Resource) error {
	sep := ""

	for _, r := range rs {
		_, err := io.WriteString(w, sep)
		if err != nil {
			return err
		}

		buf, err := proto.Marshal(r)
		if err != nil {
			return err
		}

		encoded := base64Codec.EncodeToString(buf)
		_, err = io.WriteString(w, encoded)
		if err != nil {
			return err
		}

		sep = ","
	}

	return nil
}

func DecodeResources(r io.Reader) ([]*mesosproto.Resource, error) {
	var buf bytes.Buffer
	buf.ReadFrom(r)
	encoded := strings.Split(buf.String(), ",")

	rs := make([]*mesosproto.Resource, 0, len(encoded))
	for _, e := range encoded {
		decoded, err := base64Codec.DecodeString(e)
		if err != nil {
			return nil, err
		}

		r := mesosproto.Resource{}
		if err := proto.Unmarshal(decoded, &r); err != nil {
			return nil, err
		}

		rs = append(rs, &r)
	}

	return rs, nil
}
