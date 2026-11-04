package main

import (
	"context"
	"log"
	"net/http"
	"os"
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

	mux.Handle("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	log.Println("Swagger: http://localhost:8080/swagger/index.html")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
