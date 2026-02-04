package coder

import "sutext.github.io/cable/internal/pool"

var _pool = newPool()

type encPool struct {
	p *pool.Pool[*encoder]
}

func newPool() encPool {
	return encPool{
		p: pool.New(func() *encoder {
			return &encoder{
				buf: make([]byte, 0, 256),
			}
		}),
	}
}

func (p encPool) get() *encoder {
	e := p.p.Get()
	e.pool = p
	return e
}

func (p encPool) put(e *encoder) {
	p.p.Put(e)
}
