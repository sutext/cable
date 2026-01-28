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
	// HSet accepts values in following formats:
	//
	//   - HSet(ctx, "myhash", "key1", "value1", "key2", "value2")
	//
	//   - HSet(ctx, "myhash", []string{"key1", "value1", "key2", "value2"})
	//
	//   - HSet(ctx, "myhash", map[string]interface{}{"key1": "value1", "key2": "value2"})
	//
	//     Playing struct With "redis" tag.
	//     type MyHash struct { Key1 string `redis:"key1"`; Key2 int `redis:"key2"` }
	//
	//   - HSet(ctx, "myhash", MyHash{"value1", "value2"}) Warn: redis-server >= 4.0
	//
	//     For struct, can be a structure pointer type, we only parse the field whose tag is redis.
	//     if you don't want the field to be read, you can use the `redis:"-"` flag to ignore it,
	//     or you don't need to set the redis tag.
	//     For the type of structure field, we only support simple data types:
	//     string, int/uint(8,16,32,64), float(32,64), time.Time(to RFC3339Nano), time.Duration(to Nanoseconds ),
	//     if you are other more complex or custom data types, please implement the encoding.BinaryMarshaler interface.
	//
	// Note that in older versions of Redis server(redis-server < 4.0), HSet only supports a single key-value pair.
	// redis-docs: https://redis.io/commands/hset (Starting with Redis version 4.0.0: Accepts multiple field and value arguments.)
	// If you are using a Struct type and the number of fields is greater than one,
	// you will receive an error similar to "ERR wrong number of arguments", you can use HMSet as a substitute.
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
