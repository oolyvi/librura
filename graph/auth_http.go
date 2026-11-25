package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"example/booklibrary/security"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

type authRequest struct {
	Name     string `json:"name"`
	Nick     string `json:"nick"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
type authResponse struct {
	Token string      `json:"token"`
	User  *publicUser `json:"user"`
}
type publicUser struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	Name  string `json:"name"`
	Nick  string `json:"nick"`
	Email string `json:"email"`
}

func publicUserFromDoc(d UserDocument) *publicUser {
	return &publicUser{ID: d.ID.Hex(), Name: d.Name, Nick: d.Nick, Email: d.Email, Role: d.Role}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func RegisterHandler(db *mongo.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in authRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		in.Nick = strings.TrimSpace(in.Nick)
		in.Email = strings.ToLower(strings.TrimSpace(in.Email))
		if in.Name == "" || in.Nick == "" {
			http.Error(w, "name and nick are required", http.StatusBadRequest)
			return
		}
		if err := firstError(security.MaxLen("name", in.Name, maxNameLen), security.MaxLen("nick", in.Nick, maxNickLen), security.MaxLen("email", in.Email, 254)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := mail.ParseAddress(in.Email); err != nil {
			http.Error(w, "valid email is required", http.StatusBadRequest)
			return
		}
		hash, err := hashPassword(in.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		users := db.Collection("users")
		var exists UserDocument
		err = users.FindOne(r.Context(), bson.M{"$or": bson.A{bson.M{"email": in.Email}, bson.M{"nick": bson.M{"$regex": "^" + regexp.QuoteMeta(in.Nick) + "$", "$options": "i"}}}}).Decode(&exists)
		if err == nil {
			http.Error(w, "email or nick is already registered", http.StatusConflict)
			return
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		doc := UserDocument{Name: in.Name, Nick: in.Nick, Email: in.Email, PasswordHash: hash, Role: RoleUser, CreatedAt: now, UpdatedAt: now}
		res, err := users.InsertOne(r.Context(), doc)
		if err != nil {
			http.Error(w, "could not create account", http.StatusConflict)
			return
		}
		doc.ID = res.InsertedID.(bson.ObjectID)
		token, err := signJWT(doc.ID.Hex(), doc.Role)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, authResponse{Token: token, User: publicUserFromDoc(doc)})
	}
}
func LoginHandler(db *mongo.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in authRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		var doc UserDocument
		err := db.Collection("users").FindOne(r.Context(), bson.M{"email": strings.ToLower(strings.TrimSpace(in.Email))}).Decode(&doc)
		if err != nil {
			verifyPassword(dummyPasswordHash, in.Password)
		}
		if err != nil || !verifyPassword(doc.PasswordHash, in.Password) {
			http.Error(w, "invalid email or password", http.StatusUnauthorized)
			return
		}
		if doc.Role == "" {
			doc.Role = RoleUser
		}
		token, err := signJWT(doc.ID.Hex(), doc.Role)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, authResponse{Token: token, User: publicUserFromDoc(doc)})
	}
}

func EnsureAdminAccount(db *mongo.Database) error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" && password == "" {
		return nil // Admin bootstrap is optional for local development.
	}
	if email == "" || password == "" {
		return errors.New("both ADMIN_EMAIL and ADMIN_PASSWORD must be set")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return errors.New("ADMIN_EMAIL must be a valid email")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("invalid ADMIN_PASSWORD: %w", err)
	}
	users := db.Collection("users")
	var existing UserDocument
	err = users.FindOne(context.Background(), bson.M{"email": email}).Decode(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	name := strings.TrimSpace(os.Getenv("ADMIN_NAME"))
	nick := strings.TrimSpace(os.Getenv("ADMIN_NICK"))
	if name == "" {
		name = "Administrator"
	}
	if nick == "" {
		nick = "admin"
	}
	now := time.Now().UTC()
	_, err = users.InsertOne(context.Background(), UserDocument{
		Name: name, Nick: nick, Email: email, PasswordHash: hash,
		Role: RoleAdmin, CreatedAt: now, UpdatedAt: now,
	})
	return err
}

var dummyPasswordHash = func() string {
	h, _ := bcrypt.GenerateFromPassword([]byte("timing-equalisation-only"), bcrypt.DefaultCost)
	return string(h)
}()
