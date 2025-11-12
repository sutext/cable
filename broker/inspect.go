package broker

type Inspect struct {
	ID       string         `json:"id"`
	Clients  int            `json:"clients"`
	Channels map[string]int `json:"channels"`
}

func NewInspect() *Inspect {
	return &Inspect{
		Channels: make(map[string]int),
	}
}
func (i *Inspect) merge(o *Inspect) {
	i.ID = "all"
	i.Clients += o.Clients
	for k, v := range o.Channels {
		if _, ok := i.Channels[k]; !ok {
			i.Channels[k] = 0
		}
		i.Channels[k] += v
	}
}
