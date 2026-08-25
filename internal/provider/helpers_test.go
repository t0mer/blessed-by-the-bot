package provider_test

import (
	"encoding/json"
	"net/http"
)

func decodeJSON(r *http.Request, dest any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(dest)
}
