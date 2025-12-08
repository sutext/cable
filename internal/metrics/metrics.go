package metrics

type Metrics interface {
	Inc(key string)
}
