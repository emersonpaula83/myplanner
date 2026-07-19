package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type FonteDados struct {
	ID                 uuid.UUID       `json:"id" db:"id"`
	Nome               string          `json:"nome" db:"nome"`
	Tipo               string          `json:"tipo" db:"tipo"`
	BaseURL            string          `json:"base_url" db:"base_url"`
	AuthType           string          `json:"auth_type" db:"auth_type"`
	APIToken           *string         `json:"-" db:"api_token"`
	UserEmail          *string         `json:"user_email" db:"user_email"`
	OAuth2ClientID     *string         `json:"-" db:"oauth2_client_id"`
	OAuth2ClientSecret *string         `json:"-" db:"oauth2_client_secret"`
	OAuth2AccessToken  *string         `json:"-" db:"oauth2_access_token"`
	OAuth2RefreshToken *string         `json:"-" db:"oauth2_refresh_token"`
	OAuth2TokenExpiry  *time.Time      `json:"-" db:"oauth2_token_expiry"`
	CustomFieldMap     json.RawMessage `json:"custom_field_map" db:"custom_field_map"`
	Ativo              bool            `json:"ativo" db:"ativo"`
	UltimoSync         *time.Time      `json:"ultimo_sync" db:"ultimo_sync"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
}

type Membro struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	FonteDadosID  uuid.UUID  `json:"fonte_dados_id" db:"fonte_dados_id"`
	JiraAccountID string     `json:"jira_account_id" db:"jira_account_id"`
	Nome          string     `json:"nome" db:"nome"`
	Email         *string    `json:"email" db:"email"`
	AvatarURL     *string    `json:"avatar_url" db:"avatar_url"`
	Team          *string    `json:"team" db:"team"`
	Ativo         bool       `json:"ativo" db:"ativo"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

type Projeto struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	FonteDadosID uuid.UUID  `json:"fonte_dados_id" db:"fonte_dados_id"`
	JiraID       string     `json:"jira_id" db:"jira_id"`
	Chave        string     `json:"chave" db:"chave"`
	Nome         string     `json:"nome" db:"nome"`
	Descricao    *string    `json:"descricao" db:"descricao"`
	LeadID       *uuid.UUID `json:"lead_id" db:"lead_id"`
	Categoria    *string    `json:"categoria" db:"categoria"`
	Ativo        bool       `json:"ativo" db:"ativo"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

type Sprint struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	FonteDadosID  uuid.UUID  `json:"fonte_dados_id" db:"fonte_dados_id"`
	ProjetoID     *uuid.UUID `json:"projeto_id" db:"projeto_id"`
	JiraID        int        `json:"jira_id" db:"jira_id"`
	Nome          string     `json:"nome" db:"nome"`
	Estado        *string    `json:"estado" db:"estado"`
	DataInicio    *time.Time `json:"data_inicio" db:"data_inicio"`
	DataFim       *time.Time `json:"data_fim" db:"data_fim"`
	DataConclusao *time.Time `json:"data_conclusao" db:"data_conclusao"`
	BoardID       *int       `json:"board_id" db:"board_id"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

type Produto struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	FonteDadosID uuid.UUID  `json:"fonte_dados_id" db:"fonte_dados_id"`
	JiraID       string     `json:"jira_id" db:"jira_id"`
	Nome         string     `json:"nome" db:"nome"`
	Descricao    *string    `json:"descricao" db:"descricao"`
	ProjetoID    *uuid.UUID `json:"projeto_id" db:"projeto_id"`
	Ativo        bool       `json:"ativo" db:"ativo"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

type Tarefa struct {
	ID                uuid.UUID         `json:"id" db:"id"`
	FonteDadosID      uuid.UUID         `json:"fonte_dados_id" db:"fonte_dados_id"`
	ProjetoID         uuid.UUID         `json:"projeto_id" db:"projeto_id"`
	JiraID            string            `json:"jira_id" db:"jira_id"`
	NumeroTicket      string            `json:"numero_ticket" db:"numero_ticket"`
	Resumo            string            `json:"resumo" db:"resumo"`
	Tipo              string            `json:"tipo" db:"tipo"`
	Status            string            `json:"status" db:"status"`
	Prioridade        *string           `json:"prioridade" db:"prioridade"`
	EstimativaPontos  *float64          `json:"estimativa_pontos" db:"estimativa_pontos"`
	EstimativaTempo   *int              `json:"estimativa_tempo" db:"estimativa_tempo"`
	TempoGasto        *int              `json:"tempo_gasto" db:"tempo_gasto"`
	ResponsavelID     *uuid.UUID        `json:"responsavel_id" db:"responsavel_id"`
	RelatorID         *uuid.UUID        `json:"relator_id" db:"relator_id"`
	Team              *string           `json:"team" db:"team"`
	SprintID          *uuid.UUID        `json:"sprint_id" db:"sprint_id"`
	DataCriacao       time.Time         `json:"data_criacao" db:"data_criacao"`
	DataLimite        *pgtype.Date      `json:"data_limite" db:"data_limite"`
	DataResolvido     *time.Time        `json:"data_resolvido" db:"data_resolvido"`
	DataAtualizado    *time.Time        `json:"data_atualizado" db:"data_atualizado"`
	TipoDemanda       *string           `json:"tipo_demanda" db:"tipo_demanda"`
	DataComponente    *pgtype.Date      `json:"data_componente" db:"data_componente"`
	StatusCategoria   *string           `json:"status_categoria" db:"status_categoria"`
	CamposExtras      json.RawMessage   `json:"campos_extras" db:"campos_extras"`
	CreatedAt         time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at" db:"updated_at"`

	ParentID           *uuid.UUID `json:"parent_id" db:"parent_id"`
	Apelido            *string    `json:"apelido" db:"apelido"`
	DataInicioExecucao *time.Time `json:"data_inicio_execucao" db:"data_inicio_execucao"`
}

type Disponibilidade struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	MembroID   uuid.UUID  `json:"membro_id" db:"membro_id"`
	Tipo       string     `json:"tipo" db:"tipo"`
	DataInicio pgtype.Date `json:"data_inicio" db:"data_inicio"`
	DataFim    pgtype.Date `json:"data_fim" db:"data_fim"`
	Descricao  *string    `json:"descricao" db:"descricao"`
	CriadoPor  *string    `json:"criado_por" db:"criado_por"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}

type LimiteAlerta struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	Nome          string     `json:"nome" db:"nome"`
	Descricao     *string    `json:"descricao" db:"descricao"`
	Escopo        string     `json:"escopo" db:"escopo"`
	ReferenciaID  *uuid.UUID `json:"referencia_id" db:"referencia_id"`
	Metrica       string     `json:"metrica" db:"metrica"`
	LimiteVerde   float64    `json:"limite_verde" db:"limite_verde"`
	LimiteAmarelo float64    `json:"limite_amarelo" db:"limite_amarelo"`
	LimiteLaranja float64    `json:"limite_laranja" db:"limite_laranja"`
	Padrao        bool       `json:"padrao" db:"padrao"`
	Ativo         bool       `json:"ativo" db:"ativo"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

type MembroStats struct {
	TotalTarefas        int     `json:"total_tarefas"`
	TarefasConcluidas   int     `json:"tarefas_concluidas"`
	TarefasEmAndamento  int     `json:"tarefas_em_andamento"`
	DiasAusenteAno      int     `json:"dias_ausente_ano"`
	TotalHorasEstimadas float64 `json:"total_horas_estimadas"`
}

type JiraProjectInfo struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type SyncLog struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	FonteDadosID   uuid.UUID       `json:"fonte_dados_id" db:"fonte_dados_id"`
	Tipo           string          `json:"tipo" db:"tipo"`
	Status         string          `json:"status" db:"status"`
	IniciadoEm     time.Time       `json:"iniciado_em" db:"iniciado_em"`
	FinalizadoEm   *time.Time      `json:"finalizado_em" db:"finalizado_em"`
	TotalProjetos  int             `json:"total_projetos" db:"total_projetos"`
	TotalTarefas   int             `json:"total_tarefas" db:"total_tarefas"`
	TotalMembros   int             `json:"total_membros" db:"total_membros"`
	TotalSprints   int             `json:"total_sprints" db:"total_sprints"`
	Erros          json.RawMessage `json:"erros" db:"erros"`
	Mensagem       *string         `json:"mensagem" db:"mensagem"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
}
