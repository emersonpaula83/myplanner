package domain

import (
	"time"

	"github.com/google/uuid"
)

type Usuario struct {
	ID           uuid.UUID `json:"id" db:"id"`
	NomeCompleto string    `json:"nome_completo" db:"nome_completo"`
	Apelido      string    `json:"apelido" db:"apelido"`
	Email        string    `json:"email" db:"email"`
	SenhaHash    string    `json:"-" db:"senha_hash"`
	Cargo        string    `json:"cargo" db:"cargo"`
	Ativo        bool      `json:"ativo" db:"ativo"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type LoginRequest struct {
	Email string `json:"email"`
	Senha string `json:"senha"`
}

type LoginResponse struct {
	Token   string  `json:"token"`
	Usuario Usuario `json:"usuario"`
}

type CriarUsuarioRequest struct {
	NomeCompleto string `json:"nome_completo"`
	Apelido      string `json:"apelido"`
	Email        string `json:"email"`
	Senha        string `json:"senha"`
	Cargo        string `json:"cargo"`
}

type AtualizarUsuarioRequest struct {
	NomeCompleto *string `json:"nome_completo"`
	Apelido      *string `json:"apelido"`
	Email        *string `json:"email"`
	Cargo        *string `json:"cargo"`
	Ativo        *bool   `json:"ativo"`
}

type AlterarSenhaRequest struct {
	SenhaAtual string `json:"senha_atual"`
	NovaSenha  string `json:"nova_senha"`
}

type AlcadaProjetosRequest struct {
	ProjetoIDs []uuid.UUID `json:"projeto_ids"`
}

type ProjetoResumo struct {
	ID    uuid.UUID `json:"id"`
	Chave string    `json:"chave"`
	Nome  string    `json:"nome"`
}
