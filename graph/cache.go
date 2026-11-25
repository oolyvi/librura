package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"example/booklibrary/graph/model"
	"github.com/redis/go-redis/v9"
)

const bookCacheTTL = 60 * time.Second

func (r *Resolver) cachedBooks(ctx context.Context, key string, load func() ([]*model.Book, error)) ([]*model.Book, error) {
	if r.Redis != nil {
		value, err := r.Redis.Get(ctx, key).Bytes()
		if err == nil {
			var books []*model.Book
			if json.Unmarshal(value, &books) == nil {
				return books, nil
			}
			log.Printf("Redis cache contains invalid data key=%s", key)
		} else if err != redis.Nil {
			log.Printf("Redis cache read failed: %v", err)
		}
	}

	books, err := load()
	if err != nil {
		return nil, err
	}
	if r.Redis != nil {
		if value, marshalErr := json.Marshal(books); marshalErr == nil {
			if err := r.Redis.Set(ctx, key, value, bookCacheTTL).Err(); err != nil {
				log.Printf("Redis cache write failed: %v", err)
			}
		}
	}
	return books, nil
}

func (r *Resolver) cachedBook(ctx context.Context, id string, load func() (*model.Book, error)) (*model.Book, error) {
	key := "book:item:" + id
	if r.Redis != nil {
		value, err := r.Redis.Get(ctx, key).Bytes()
		if err == nil {
			var book model.Book
			if json.Unmarshal(value, &book) == nil {
				return &book, nil
			}
		} else if err != redis.Nil {
			log.Printf("Redis cache read failed: %v", err)
		}
	}

	book, err := load()
	if err != nil || book == nil {
		return book, err
	}
	if r.Redis != nil {
		if value, marshalErr := json.Marshal(book); marshalErr == nil {
			if err := r.Redis.Set(ctx, key, value, bookCacheTTL).Err(); err != nil {
				log.Printf("Redis cache write failed: %v", err)
			}
		}
	}
	return book, nil
}

func (r *Resolver) invalidateBookCache(ctx context.Context) {
	if r.Redis == nil {
		return
	}
	var cursor uint64
	for {
		keys, next, err := r.Redis.Scan(ctx, cursor, "book:*", 100).Result()
		if err != nil {
			log.Printf("Redis cache invalidation scan failed: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := r.Redis.Del(ctx, keys...).Err(); err != nil {
				log.Printf("Redis cache invalidation failed: %v", err)
				return
			}
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

func bookListCacheKey(filter any) string {
	value, err := json.Marshal(filter)
	if err != nil {
		return fmt.Sprintf("book:list:%v", filter)
	}
	return "book:list:" + string(value)
}

func ConnectRedis(url string) *redis.Client {
	options, err := redis.ParseURL(url)
	if err != nil {
		log.Fatal("Invalid REDIS_URL:", err)
	}
	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("Redis unavailable; caching disabled: %v", err)
		_ = client.Close()
		return nil
	}
	log.Println("Connected to Redis!")
	return client
}
