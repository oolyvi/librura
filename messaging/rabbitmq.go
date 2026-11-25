package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sony/gobreaker"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Publisher struct {
	url     string
	conn    *amqp.Connection
	ch      *amqp.Channel
	mu      sync.Mutex
	breaker *gobreaker.CircuitBreaker
}

func NewPublisher(url string) (*Publisher, error) {
	p := &Publisher{url: url}

	p.mu.Lock()
	err := p.connectLocked()
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}

	p.breaker = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "rabbitmq-publisher",
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Printf("circuit breaker %q changed state: %s -> %s", name, from, to)
		},
	})

	return p, nil
}

func (p *Publisher) connectLocked() error {
	p.closeConnectionLocked()

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}

	if err := ch.ExchangeDeclare(ExchangeName, "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("declare exchange: %w", err)
	}
	q, err := ch.QueueDeclare(QueueName, true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("declare activity queue: %w", err)
	}
	if err := ch.QueueBind(q.Name, "book.#", ExchangeName, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("bind activity queue: %w", err)
	}

	p.conn = conn
	p.ch = ch
	log.Printf("RabbitMQ publisher connected")
	return nil
}

func (p *Publisher) closeConnectionLocked() {
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

func (p *Publisher) ensureConnectedLocked() error {
	if p.conn != nil && !p.conn.IsClosed() && p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	log.Printf("RabbitMQ publisher connection is closed; reconnecting")
	return p.connectLocked()
}

func (p *Publisher) Publish(event Event) error {
	body, err := event.JSON()
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	_, err = p.breaker.Execute(func() (interface{}, error) {
		if err := p.ensureConnectedLocked(); err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		publishErr := p.ch.PublishWithContext(ctx, ExchangeName, event.Type, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.ID,
			Timestamp:    event.OccurredAt,
			Body:         body,
		})
		if publishErr != nil {
			p.closeConnectionLocked()
			return nil, publishErr
		}
		return nil, nil
	})
	return err
}

func (p *Publisher) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeConnectionLocked()
}

func StartConsumer(url string, db *mongo.Database) {
	go func() {
		for {
			if err := consume(url, db); err != nil {
				log.Printf("RabbitMQ consumer stopped: %v; retrying in 5 seconds", err)
				time.Sleep(5 * time.Second)
			}
		}
	}()
}

func consume(url string, db *mongo.Database) error {
	conn, err := amqp.Dial(url)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(ExchangeName, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	q, err := ch.QueueDeclare(QueueName, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(q.Name, "book.#", ExchangeName, false, nil); err != nil {
		return err
	}
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}

	deliveries, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	log.Printf("RabbitMQ worker consuming queue %q", q.Name)

	for delivery := range deliveries {
		var event Event
		if err := json.Unmarshal(delivery.Body, &event); err != nil {
			log.Printf("Invalid RabbitMQ event: %v", err)
			_ = delivery.Nack(false, false)
			continue
		}

		_, err := db.Collection("activity_events").UpdateOne(
			context.Background(),
			bson.M{"_id": event.ID},
			bson.M{"$setOnInsert": bson.M{
				"type":       event.Type,
				"entityId":   event.EntityID,
				"occurredAt": event.OccurredAt,
				"data":       event.Data,
			}},
			options.UpdateOne().SetUpsert(true),
		)
		if err != nil {
			_ = delivery.Nack(false, true)
			return fmt.Errorf("persist activity event: %w", err)
		}
		log.Printf("Stored activity event type=%s entityId=%s eventId=%s", event.Type, event.EntityID, event.ID)
		if err := delivery.Ack(false); err != nil {
			return err
		}
	}
	return fmt.Errorf("RabbitMQ delivery channel closed")
}
