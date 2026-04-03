package dbnodes

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// RedisNode executes Redis operations.
// Type: "db.redis"
type RedisNode struct{}

func (n *RedisNode) Type() string { return "db.redis" }

func (n *RedisNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	operation, _ := config["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("db.redis: 'operation' is required")
	}

	addr, _ := config["addr"].(string)
	if addr == "" {
		addr = "localhost:6379"
	}

	password, _ := config["password"].(string)

	dbIndex := 0
	if v, ok := config["db"].(float64); ok {
		dbIndex = int(v)
	}

	key, _ := config["key"].(string)

	ttlSecs := 0
	if v, ok := config["ttl_seconds"].(float64); ok {
		ttlSecs = int(v)
	}
	var ttl time.Duration
	if ttlSecs > 0 {
		ttl = time.Duration(ttlSecs) * time.Second
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       dbIndex,
	})
	defer rdb.Close()

	var resultItem workflow.Item

	switch operation {
	case "get":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for get")
		}
		val, err := rdb.Get(ctx, key).Result()
		if err == redis.Nil {
			resultItem = workflow.NewItem(map[string]interface{}{"key": key, "value": nil, "exists": false})
		} else if err != nil {
			return nil, fmt.Errorf("db.redis: get failed: %w", err)
		} else {
			resultItem = workflow.NewItem(map[string]interface{}{"key": key, "value": val, "exists": true})
		}

	case "set":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for set")
		}
		value := config["value"]
		if err := rdb.Set(ctx, key, fmt.Sprintf("%v", value), ttl).Err(); err != nil {
			return nil, fmt.Errorf("db.redis: set failed: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "success": true})

	case "del":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for del")
		}
		count, err := rdb.Del(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("db.redis: del failed: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "deleted_count": count})

	case "exists":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for exists")
		}
		count, err := rdb.Exists(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("db.redis: exists failed: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "exists": count > 0})

	case "expire":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for expire")
		}
		if ttl == 0 {
			return nil, fmt.Errorf("db.redis: 'ttl_seconds' is required for expire")
		}
		ok, err := rdb.Expire(ctx, key, ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("db.redis: expire failed: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "success": ok})

	case "lpush":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for lpush")
		}
		value := config["value"]
		length, err := rdb.LPush(ctx, key, fmt.Sprintf("%v", value)).Result()
		if err != nil {
			return nil, fmt.Errorf("db.redis: lpush failed: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "length": length})

	case "lrange":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for lrange")
		}
		start := int64(0)
		stop := int64(-1)
		if v, ok := config["start"].(float64); ok {
			start = int64(v)
		}
		if v, ok := config["stop"].(float64); ok {
			stop = int64(v)
		}
		vals, err := rdb.LRange(ctx, key, start, stop).Result()
		if err != nil {
			return nil, fmt.Errorf("db.redis: lrange failed: %w", err)
		}
		ivals := make([]interface{}, len(vals))
		for i, v := range vals {
			ivals[i] = v
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "values": ivals})

	case "hset":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for hset")
		}
		field, _ := config["field"].(string)
		value := config["value"]
		if field == "" {
			return nil, fmt.Errorf("db.redis: 'field' is required for hset")
		}
		if err := rdb.HSet(ctx, key, field, fmt.Sprintf("%v", value)).Err(); err != nil {
			return nil, fmt.Errorf("db.redis: hset failed: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "field": field, "success": true})

	case "hget":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for hget")
		}
		field, _ := config["field"].(string)
		if field == "" {
			return nil, fmt.Errorf("db.redis: 'field' is required for hget")
		}
		val, err := rdb.HGet(ctx, key, field).Result()
		if err == redis.Nil {
			resultItem = workflow.NewItem(map[string]interface{}{"key": key, "field": field, "value": nil, "exists": false})
		} else if err != nil {
			return nil, fmt.Errorf("db.redis: hget failed: %w", err)
		} else {
			resultItem = workflow.NewItem(map[string]interface{}{"key": key, "field": field, "value": val, "exists": true})
		}

	case "hgetall":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for hgetall")
		}
		vals, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("db.redis: hgetall failed: %w", err)
		}
		data := make(map[string]interface{}, len(vals))
		for k, v := range vals {
			data[k] = v
		}
		data["_key"] = key
		resultItem = workflow.NewItem(data)

	case "keys":
		pattern, _ := config["key"].(string)
		if pattern == "" {
			pattern = "*"
		}
		var keys []string
		var cursor uint64
		for {
			var batch []string
			var err error
			batch, cursor, err = rdb.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				return nil, fmt.Errorf("db.redis: keys (scan) failed: %w", err)
			}
			keys = append(keys, batch...)
			if cursor == 0 {
				break
			}
		}
		ikeys := make([]interface{}, len(keys))
		for i, k := range keys {
			ikeys[i] = k
		}
		resultItem = workflow.NewItem(map[string]interface{}{"keys": ikeys})

	case "incr":
		if key == "" {
			return nil, fmt.Errorf("db.redis: 'key' is required for incr")
		}
		val, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("db.redis: incr failed: %w", err)
		}
		resultItem = workflow.NewItem(map[string]interface{}{"key": key, "value": val})

	default:
		return nil, fmt.Errorf("db.redis: unknown operation %q", operation)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{resultItem}}}, nil
}
