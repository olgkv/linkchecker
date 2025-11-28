package domain

type LinkStatus string

const (
	StatusAvailable    LinkStatus = "available"
	StatusNotAvailable LinkStatus = "not available"
)

type Task struct {
	ID     int               `json:"id"`
	Links  []string          `json:"links"`
	Result map[string]string `json:"result"`
}
