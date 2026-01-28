package main

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisBegin struct {
	Cmd       string
	BeginTime time.Time
}

type RedisEnd struct {
	Cmd       string
	Error     error
	EndTime   time.Time
	BeginTime time.Time
}

type redisHandler interface {
	RedisBegin(ctx context.Context, info *RedisBegin) context.Context
	RedisEnd(ctx context.Context, info *RedisEnd)
}

type redisImpl interface {
	Close() error
	HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd
	HSet(ctx context.Context, key string, values ...any) *redis.IntCmd
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
}

type redisClient struct {
	impl    redisImpl
	handler redisHandler
}

func newRedis(config *config, handler redisHandler) *redisClient {
	if config.Redis.Enabled {
		cli := redis.NewClient(&redis.Options{
			Addr:     config.Redis.Address,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
		})
		return &redisClient{impl: cli, handler: handler}
	}
	if config.RedisCluster.Enabled {
		cli := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    config.RedisCluster.Addresses,
			Password: config.RedisCluster.Password,
		})
		return &redisClient{impl: cli, handler: handler}
	}
	return nil
}
func (c *redisClient) Close() error {
	return c.impl.Close()
}
func (c *redisClient) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	beginTime := time.Now()
	ctx = c.handler.RedisBegin(ctx, &RedisBegin{
		Cmd:       "HDel",
		BeginTime: beginTime,
	})
	i, err := c.impl.HDel(ctx, key, fields...).Result()
	c.handler.RedisEnd(ctx, &RedisEnd{
		Cmd:       "HDel",
		BeginTime: beginTime,
		EndTime:   time.Now(),
		Error:     err,
	})
	return i, err
}
func (c *redisClient) HSet(ctx context.Context, key string, values ...any) (int64, error) {
	beginTime := time.Now()
	ctx = c.handler.RedisBegin(ctx, &RedisBegin{
		Cmd:       "HSet",
		BeginTime: beginTime,
	})
	i, err := c.impl.HSet(ctx, key, values...).Result()
	c.handler.RedisEnd(ctx, &RedisEnd{
		Cmd:       "HSet",
		BeginTime: beginTime,
		EndTime:   time.Now(),
		Error:     err,
	})
	return i, err
}
func (c *redisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	beginTime := time.Now()
	ctx = c.handler.RedisBegin(ctx, &RedisBegin{
		Cmd:       "HGetAll",
		BeginTime: beginTime,
	})
	m, err := c.impl.HGetAll(ctx, key).Result()
	c.handler.RedisEnd(ctx, &RedisEnd{
		Cmd:       "HGetAll",
		BeginTime: beginTime,
		EndTime:   time.Now(),
		Error:     err,
	})
	return m, err
}
