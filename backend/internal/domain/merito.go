package domain

import (
	"time"

	"github.com/google/uuid"
)

type HistoricoMeritoPromocao struct {
	ID              uuid.UUID `json:"id"`
	MembroID        uuid.UUID `json:"membro_id"`
	Tipo            string    `json:"tipo"`
	CargoAnterior   *string   `json:"cargo_anterior"`
	CargoNovo       *string   `json:"cargo_novo"`
	SalarioAnterior *float64  `json:"salario_anterior"`
	SalarioNovo     float64   `json:"salario_novo"`
	DataVigencia    time.Time `json:"data_vigencia"`
	CreatedAt       time.Time `json:"created_at"`
}

type MeritoPromocaoRequest struct {
	Tipo         string  `json:"tipo"`
	CargoNovo    *string `json:"cargo_novo"`
	SalarioNovo  float64 `json:"salario_novo"`
	DataVigencia string  `json:"data_vigencia"`
}

type MembroSnapshot struct {
	Cargo   *string  `json:"cargo"`
	Salario *float64 `json:"salario"`
}

type MeritoPromocaoResponse struct {
	HistoricoID uuid.UUID      `json:"historico_id"`
	Antes       MembroSnapshot `json:"antes"`
	Depois      MembroSnapshot `json:"depois"`
}

type MembroComEntrada struct {
	Membro      Membro    `json:"membro"`
	DataEntrada time.Time `json:"data_entrada"`
}

type TransferConflict struct {
	Conflito    bool `json:"conflito"`
	EquipeAtual struct {
		ID   uuid.UUID `json:"id"`
		Nome string    `json:"nome"`
	} `json:"equipe_atual"`
}
