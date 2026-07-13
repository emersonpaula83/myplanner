package domain

import (
	"time"

	"github.com/google/uuid"
)

type TimelineResponse struct {
	Equipe           string            `json:"equipe"`
	Ano              int               `json:"ano"`
	Projetos         []ProjetoTimeline `json:"projetos"`
	CapacidadeMensal []CapacidadeMes   `json:"capacidade_mensal"`
}

type ProjetoTimeline struct {
	ID                 uuid.UUID  `json:"id"`
	NumeroTicket       string     `json:"numero_ticket"`
	Apelido            *string    `json:"apelido"`
	Resumo             string     `json:"resumo"`
	TipoDemanda        *string    `json:"tipo_demanda"`
	Status             string     `json:"status"`
	DataInicioExecucao *time.Time `json:"data_inicio_execucao"`
	DataLimite         *string    `json:"data_limite"`
	TotalDiasEstimados float64    `json:"total_dias_estimados"`
	ProjetoCI          bool       `json:"projeto_ci"`
	ProjetoCITicket    *string    `json:"projeto_ci_ticket"`
}

type CapacidadeMes struct {
	Mes              int             `json:"mes"`
	HorasDisponiveis float64         `json:"horas_disponiveis"`
	HorasEstimadas   float64         `json:"horas_estimadas"`
	PercentualDelta  float64         `json:"percentual_delta"`
	MembrosAusentes  []MembroAusente `json:"membros_ausentes"`
}

type MembroAusente struct {
	Nome string `json:"nome"`
	Tipo string `json:"tipo"`
	Dias int    `json:"dias"`
}

type ProjetoListItem struct {
	ID                 uuid.UUID  `json:"id"`
	NumeroTicket       string     `json:"numero_ticket"`
	Resumo             string     `json:"resumo"`
	Apelido            *string    `json:"apelido"`
	DataInicioExecucao *time.Time `json:"data_inicio_execucao"`
	DataLimite         *string    `json:"data_limite"`
	TipoDemanda        *string    `json:"tipo_demanda"`
	Status             string     `json:"status"`
}

type AnaliseResponse struct {
	Analise string `json:"analise"`
}

type MetadataProjetoRequest struct {
	Apelido            *string    `json:"apelido"`
	DataInicioExecucao *time.Time `json:"data_inicio_execucao"`
}

type AnalisarCapacidadeRequest struct {
	Equipe string `json:"equipe"`
	Ano    int    `json:"ano"`
	Mes    int    `json:"mes"`
}

type EpicoEquipe struct {
	ID                  uuid.UUID
	NumeroTicket        string
	Resumo              string
	Status              string
	Apelido             *string
	DataInicioExecucao  *time.Time
	DataLimite          *time.Time
	TipoDemanda         *string
	TotalSegundosEquipe int64
	ProjetoCI           bool
	ProjetoCITicket     *string
}

type AusenciaMensal struct {
	MembroID uuid.UUID
	Nome     string
	Tipo     string
	Mes      int
	Dias     int
}

type ProjetoCapacidade struct {
	DataInicioExecucao time.Time
	DataLimite         time.Time
	HorasEquipe        float64
}

type AnaliseCapacidadeInput struct {
	Equipe           string
	Ano              int
	Mes              int
	HorasDisponiveis float64
	HorasEstimadas   float64
	PercentualDelta  float64
	MembrosAusentes  []MembroAusente
	Projetos         []ProjetoAnalise
}

type ProjetoAnalise struct {
	Apelido  string
	HorasMes float64
	Resumo   string
}
