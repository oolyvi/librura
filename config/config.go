package config

import (
	"bufio"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Port         string
	MongoDBURI   string
	DatabaseName string
	RabbitMQURL  string
	RedisURL     string
}

func LoadConfig() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		MongoDBURI:   getEnv("MONGODB_URI", "mongodb://localhost:27017"),
		DatabaseName: getEnv("MONGODB_DATABASE", "booklibrary"),
		RabbitMQURL:  rabbitMQURL(),
		RedisURL:     getEnv("REDIS_URL", "redis://localhost:6379/0"),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func rabbitMQURL() string {
	if v := os.Getenv("RABBITMQ_URL"); v != "" {
		return v
	}
	user, pass := os.Getenv("RABBITMQ_USER"), os.Getenv("RABBITMQ_PASS")
	if user == "" {
		user, pass = "guest", "guest"
	}
	return "amqp://" + url.UserPassword(user, pass).String() + "@localhost:5672/"
}

func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
