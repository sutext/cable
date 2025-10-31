package main

import (
	"context"
	api "sutext.github.io/cable/api/kitex_gen/api"
)

// ServiceImpl implements the last service interface defined in the IDL.
type ServiceImpl struct{}

// Join implements the ServiceImpl interface.
func (s *ServiceImpl) Join(ctx context.Context, req *api.JoinRequest) (resp *api.JoinResponse, err error) {
	// TODO: Your code here...
	return
}

// Leave implements the ServiceImpl interface.
func (s *ServiceImpl) Leave(ctx context.Context, req *api.JoinRequest) (resp *api.JoinResponse, err error) {
	// TODO: Your code here...
	return
}

// Publish implements the ServiceImpl interface.
func (s *ServiceImpl) Publish(ctx context.Context, req *api.PublishRequest) (err error) {
	// TODO: Your code here...
	return
}
