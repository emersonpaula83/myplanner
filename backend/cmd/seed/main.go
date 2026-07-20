package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/emersonpaula83/myplanner/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	passApp := cfg.PassApp
	if passApp == "" {
		log.Fatal("PASS_APP not set in .env")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(passApp), 12)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, cfg.DB.DSN())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, `
		INSERT INTO usuarios (nome_completo, apelido, email, senha_hash, cargo)
		VALUES ('Administrador', 'admin', $1, $2, 'coordenador')
		ON CONFLICT (email) DO UPDATE SET senha_hash = $2
	`, cfg.Auth.AdminEmail, string(hash))
	if err != nil {
		log.Fatalf("failed to seed admin user: %v", err)
	}

	_, err = conn.Exec(ctx, `
		INSERT INTO usuario_projetos (usuario_id, projeto_id)
		SELECT u.id, p.id
		FROM usuarios u, projetos p
		WHERE u.apelido = 'admin'
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		log.Fatalf("failed to grant admin access: %v", err)
	}

	fmt.Println("admin user seeded successfully")
}
