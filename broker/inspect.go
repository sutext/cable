package broker

type Inspect struct {
	ID       string                      `json:"id"`
	Queue    int64                       `json:"queue"`
	Clients  map[string]map[string]uint8 `json:"clients"`
	Channels map[string]map[string]uint8 `json:"channels"`
}
