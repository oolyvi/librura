package main

import (
	"example/booklibrary/config"
	"example/booklibrary/database"
	"example/booklibrary/graph"
	"example/booklibrary/messaging"
	"example/booklibrary/security"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
)

const defaultPort = "8080"

func main() {
	config.LoadDotEnv(".env")
	if err := graph.ValidateJWTSecret(); err != nil {
		log.Fatal(err)
	}
	cfg := config.LoadConfig()

	db := database.ConnectMongoDB(cfg)
	if err := graph.EnsureAdminAccount(db); err != nil {
		log.Fatal("Admin account bootstrap failed:", err)
	}

	cache := graph.ConnectRedis(cfg.RedisURL)
	if cache != nil {
		defer cache.Close()
	}

	publisher, err := messaging.NewPublisher(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal("RabbitMQ connection failed:", err)
	}
	defer publisher.Close()
	messaging.StartConsumer(cfg.RabbitMQURL, db)

	port := cfg.Port

	if port == "" {
		port = defaultPort
	}

	resolver := graph.NewResolver(db, publisher, cache)
	srv := handler.New(
		graph.NewExecutableSchema(
			graph.Config{Resolvers: resolver},
		),
	)

	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.GET{})
	srv.Use(extension.FixedComplexityLimit(200))

	enablePlayground := envBool("ENABLE_PLAYGROUND")
	trustProxy := envBool("TRUST_PROXY")
	if enablePlayground {
		srv.Use(extension.Introspection{})
	}

	authLimiter := security.NewRateLimiter(10, 5)
	apiLimiter := security.NewRateLimiter(300, 60)

	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("frontend"))))
	if enablePlayground {
		log.Println("WARNING: GraphQL playground is enabled (ENABLE_PLAYGROUND=true)")
		mux.Handle("/playground", playground.Handler("GraphQL playground", "/query"))
	}
	mux.Handle("/", http.FileServer(http.Dir("frontend")))
	mux.Handle("/auth/register", security.RateLimit(authLimiter, trustProxy, graph.RegisterHandler(db)))
	mux.Handle("/auth/login", security.RateLimit(authLimiter, trustProxy, graph.LoginHandler(db)))
	mux.Handle("/query", security.RateLimit(apiLimiter, trustProxy, graph.AuthMiddleware(db, srv)))
	mux.Handle("/admin/", security.RateLimit(apiLimiter, trustProxy, graph.AdminHandler(resolver)))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           security.Headers(security.LimitBody(1<<20, mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	log.Printf("Server running on port %s", port)
	log.Fatal(server.ListenAndServe())
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
