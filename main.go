package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sqlOrm/DB"
	"sqlOrm/User"
	_ "sqlOrm/docs"

	// Swagger dosyaları üretildikten sonra aç:
	// _ "sqlOrm/docs"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// @title SQL ORM API
// @version 1.0
// @description Generic repository ile kullanıcı arama API'si.
// @host localhost:8080
// @BasePath /
// @schemes http
func main() {
	// Yerel SQL Server sertifikası için kullandığımız uyumluluk ayarı.
	debugSettings := os.Getenv("GODEBUG")
	if debugSettings != "" {
		debugSettings += ","
	}

	if err := os.Setenv(
		"GODEBUG",
		debugSettings+"x509negativeserial=1",
	); err != nil {
		log.Fatal(err)
	}

	connectionString := os.Getenv("SQL_CONNECTION_STRING")
	if connectionString == "" {
		log.Fatal("SQL_CONNECTION_STRING belirtilmeli")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := DB.SqlOpen(ctx, connectionString)
	cancel()

	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	userRepository, err := User.NewUserRepository(db)
	if err != nil {
		log.Fatal(err)
	}

	userHandler := User.NewUserHandler(userRepository)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/users", userHandler.GetByName)
	mux.HandleFunc("/api/users/paging", userHandler.GetAllWithPaging)
	mux.HandleFunc("/api/users/search", userHandler.SearchUsers)
	mux.HandleFunc(
		"/api/users/append-x-over-40",
		userHandler.AppendXToNamesOver40,
	)
	mux.HandleFunc(
		"/api/users/permissions",
		userHandler.GetUserPermissions,
	)
	mux.HandleFunc("/api/users/bulk-insert", userHandler.InsertSampleUsers)
	mux.HandleFunc("/api/users/bulk-delete", userHandler.DeleteUsersFromID)

	mux.Handle("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// http.ListenAndServe zaman aşımı olmadan çalışır; yavaş istemciler
	// bağlantıları süresiz tutabilir.
	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	signalCtx, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	serverErrors := make(chan error, 1)

	log.Println("Swagger: http://localhost:8080/swagger/index.html")

	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		// log.Fatal defer'ları çalıştırmaz; havuzu önce kapatıyoruz.
		db.Close()
		log.Fatal(err)
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Sunucu düzgün kapatılamadı: %v", err)
		}
	}
}
