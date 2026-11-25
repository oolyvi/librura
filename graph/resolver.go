package graph

import (
	"example/booklibrary/messaging"
	"example/booklibrary/security"
	"fmt"
	"log"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"github.com/redis/go-redis/v9"
)

type Resolver struct {
	DB        *mongo.Database
	Publisher *messaging.Publisher
	Redis     *redis.Client
}

func NewResolver(db *mongo.Database, publisher *messaging.Publisher, cache *redis.Client) *Resolver {
	return &Resolver{DB: db, Publisher: publisher, Redis: cache}
}

func (r *Resolver) publishBookEvent(eventType, id string) {
	if r.Publisher == nil {
		return
	}
	if err := r.Publisher.Publish(messaging.NewEvent(eventType, id, nil)); err != nil {
		log.Printf("could not publish %s for book %s: %v", eventType, id, err)
	}
}

func validateRating(rating int32) error {
	if rating < 0 || rating > 5 {
		return fmt.Errorf("rating must be between 0 and 5, got %d", rating)
	}
	return nil
}

func validatePageCount(p int32) error {
	if p < 0 {
		return fmt.Errorf("page count must not be negative, got %d", p)
	}
	return nil
}

func validatePrice(p float64) error {
	if p < 0 {
		return fmt.Errorf("price must not be negative, got %v", p)
	}
	return nil
}

func requireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	return nil
}

const (
	maxNameLen     = 100
	maxNickLen     = 40
	maxLocationLen = 100
	maxTitleLen    = 300
	maxReviewLen   = 5000
	maxQuotes      = 50
	maxQuoteLen    = 1000
)

func firstError(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func validateQuotes(quotes []string) error {
	if len(quotes) > maxQuotes {
		return fmt.Errorf("a book can have at most %d quotes", maxQuotes)
	}
	for _, q := range quotes {
		if err := security.MaxLen("quote", q, maxQuoteLen); err != nil {
			return err
		}
	}
	return nil
}

func validateBookText(title, author, review *string, quotes []string) error {
	if title != nil {
		if err := security.MaxLen("title", *title, maxTitleLen); err != nil {
			return err
		}
	}
	if author != nil {
		if err := security.MaxLen("author", *author, maxNameLen); err != nil {
			return err
		}
	}
	if review != nil {
		if err := security.MaxLen("review", *review, maxReviewLen); err != nil {
			return err
		}
	}
	return validateQuotes(quotes)
}
