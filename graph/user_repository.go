package graph

import (
	"example/booklibrary/graph/model"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type UserDocument struct {
	ID                bson.ObjectID `bson:"_id,omitempty"`
	Name              string        `bson:"name"`
	Nick              string        `bson:"nick"`
	Email             string        `bson:"email"`
	YearlyReadingGoal int32         `bson:"yearlyReadingGoal"`
	Role              string        `bson:"role"`
	PasswordHash      string        `bson:"passwordHash"`
	Location          *string       `bson:"location,omitempty"`
	BirthDate         *string       `bson:"birthDate,omitempty"`
	Photo             *string       `bson:"photo,omitempty"`
	CreatedAt         time.Time     `bson:"createdAt"`
	UpdatedAt         time.Time     `bson:"updatedAt"`
}

func toUserModel(doc UserDocument) *model.User {
	return &model.User{
		ID: doc.ID.Hex(), Name: doc.Name, Nick: doc.Nick,
		Location: doc.Location, BirthDate: doc.BirthDate, Photo: doc.Photo,
		CreatedAt: doc.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: doc.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
