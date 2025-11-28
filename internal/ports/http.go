package ports

import "net/http"

// HTTPClient abstracts Do method used by service for outgoing HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}
