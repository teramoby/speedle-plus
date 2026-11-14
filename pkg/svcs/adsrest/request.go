// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"bytes"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"io"
	"net/http"
)

// maxAuthzRequestBodyBytes bounds the buffered authorization request.
const maxAuthzRequestBodyBytes = 1 << 20

// decodeBoundedJSONContext retains the authorization request size limit.
func decodeBoundedJSONContext(r *http.Request) (*JsonContext, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAuthzRequestBodyBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, errors.InvalidRequest, "unable to read request")
	}
	if len(body) > maxAuthzRequestBodyBytes {
		return nil, errors.New(errors.InvalidRequest, "request body is too large")
	}
	request, err := decodeJSONContext(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return request, nil
}
