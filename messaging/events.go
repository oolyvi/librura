package messaging

import (
	"encoding/json"
	"time"
)

const ExchangeName = "booklibrary.events"
const QueueName = "booklibrary.activity"

type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	EntityID   string         `json:"entityId"`
	OccurredAt time.Time      `json:"occurredAt"`
	Data       map[string]any `json:"data,omitempty"`
}

func NewEvent(eventType, entityID string, data map[string]any) Event {
	return Event{
		ID: newEventID(), Type: eventType, EntityID: entityID,
		OccurredAt: time.Now().UTC(), Data: data,
	}
}

func (e Event) JSON() ([]byte, error) { return json.Marshal(e) }
