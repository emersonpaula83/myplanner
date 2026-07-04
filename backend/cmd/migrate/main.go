package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/totvs/tcloud-planner/backend/internal/config"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	flag.Parse()

	if *direction != "up" && *direction != "down" {
		log.Fatalf("invalid direction %q: must be \"up\" or \"down\"", *direction)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	m, err := migrate.New("file://migrations", cfg.DB.DSN())
	if err != nil {
		log.Fatalf("failed to create migrate instance: %v", err)
	}
	defer m.Close()

	switch *direction {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	}

	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no changes")
		return
	}
	if err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	fmt.Printf("migration %s completed successfully\n", *direction)
}
