package broker

type Inspect struct {
	ID       string         `json:"id"`
	Queue    int64          `json:"queue"`
	Clients  map[string]int `json:"clients"`
	Channels map[string]int `json:"channels"`
}
