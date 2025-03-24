package main

import (
	"database/sql"
	"fmt"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"math/rand"
	"os"
	"time"
)

var db *sql.DB
var seeded bool = false

func init() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Connect to PostgreSQL
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_DB"),
	)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Ensure the table exists
	table := os.Getenv("POSTGRES_TABLE")

	createTableQuery := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id SERIAL PRIMARY KEY,
		tracking_id TEXT NOT NULL,
		salt TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT NOW()
	)`, table)

	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
}

func CreateTrackingId(salt string) string {
	if !seeded {
		rand.Seed(time.Now().UnixNano())
		seeded = true
	}

	trackingId := fmt.Sprintf("%c%c-%d%s-%d%s",
		getRandomLetterCode(),
		getRandomLetterCode(),
		len(salt),
		getRandomNumber(3),
		len(salt)/2,
		getRandomNumber(7),
	)

	// Store tracking ID in database
	_, err := db.Exec("INSERT INTO tracking (tracking_id, salt) VALUES ($1, $2)", trackingId, salt)
	if err != nil {
		log.Printf("Failed to insert tracking ID: %v", err)
	}

	return trackingId
}

func getRandomLetterCode() uint32 {
	return 65 + uint32(rand.Intn(25))
}

func getRandomNumber(digits int) string {
	str := ""
	for i := 0; i < digits; i++ {
		str = fmt.Sprintf("%s%d", str, rand.Intn(10))
	}
	return str
}
