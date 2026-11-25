package graph

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const (
	userIDContextKey   contextKey = "authenticatedUserID"
	userRoleContextKey contextKey = "authenticatedUserRole"
)

const (
	RoleUser  = "USER"
	RoleAdmin = "ADMIN"
)

type jwtClaims struct {
	UserID    string `json:"sub"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
}

func jwtSecret() ([]byte, error) {
	s := os.Getenv("JWT_SECRET")
	if len(s) < 32 {
		return nil, errors.New("JWT_SECRET must be set to at least 32 characters")
	}
	return []byte(s), nil
}
func signJWT(userID string, roles ...string) (string, error) {
	role := RoleUser
	if len(roles) > 0 && roles[0] != "" {
		role = roles[0]
	}
	secret, err := jwtSecret()
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	now := time.Now().Unix()
	claims, _ := json.Marshal(jwtClaims{UserID: userID, Role: role, IssuedAt: now, ExpiresAt: now + 8*60*60})
	body := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(body))
	return body + "." + enc.EncodeToString(mac.Sum(nil)), nil
}
func parseJWT(token string) (jwtClaims, error) {
	secret, err := jwtSecret()
	if err != nil {
		return jwtClaims{}, err
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, errors.New("invalid token")
	}
	headerRaw, headerErr := base64.RawURLEncoding.DecodeString(parts[0])
	if headerErr != nil {
		return jwtClaims{}, errors.New("invalid token header")
	}
	var header map[string]string
	if json.Unmarshal(headerRaw, &header) != nil || header["alg"] != "HS256" {
		return jwtClaims{}, errors.New("unsupported token algorithm")
	}
	body := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtClaims{}, errors.New("invalid token")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(body))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return jwtClaims{}, errors.New("invalid token signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, errors.New("invalid token")
	}
	var claims jwtClaims
	if err = json.Unmarshal(raw, &claims); err != nil {
		return jwtClaims{}, errors.New("invalid token claims")
	}
	if claims.UserID == "" || (claims.Role != RoleUser && claims.Role != RoleAdmin) || claims.ExpiresAt <= time.Now().Unix() {
		return jwtClaims{}, errors.New("token expired or invalid")
	}
	return claims, nil
}

func AuthMiddleware(db *mongo.Database, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}
			claims, err := parseJWT(parts[1])
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			oid, err := bson.ObjectIDFromHex(claims.UserID)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var current struct {
				Role string `bson:"role"`
			}
			err = db.Collection("users").FindOne(r.Context(), bson.M{"_id": oid}, options.FindOne().SetProjection(bson.M{"role": 1})).Decode(&current)
			if errors.Is(err, mongo.ErrNoDocuments) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			role := current.Role
			if role == "" {
				role = RoleUser
			}
			ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
			ctx = context.WithValue(ctx, userRoleContextKey, role)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}
func authenticatedUserID(ctx context.Context) (bson.ObjectID, error) {
	id, ok := ctx.Value(userIDContextKey).(string)
	if !ok || id == "" {
		return bson.ObjectID{}, errors.New("authentication required")
	}
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, errors.New("invalid authenticated user")
	}
	return oid, nil
}
func hashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 72 {
		return "", fmt.Errorf("password must not exceed 72 bytes")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}
func verifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func ValidateJWTSecret() error {
	_, err := jwtSecret()
	return err
}

func authenticatedRole(ctx context.Context) (string, error) {
	role, ok := ctx.Value(userRoleContextKey).(string)
	if !ok || (role != RoleUser && role != RoleAdmin) {
		return "", errors.New("authentication required")
	}
	return role, nil
}

func requireAdmin(ctx context.Context) error {
	role, err := authenticatedRole(ctx)
	if err != nil {
		return err
	}
	if role != RoleAdmin {
		return errors.New("admin access required")
	}
	return nil
}

func isAdmin(ctx context.Context) bool {
	role, _ := authenticatedRole(ctx)
	return role == RoleAdmin
}
