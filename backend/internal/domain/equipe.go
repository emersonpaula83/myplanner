package domain

import (
	"time"

	"github.com/google/uuid"
)

type Equipe struct {
	ID        uuid.UUID `json:"id"`
	Nome      string    `json:"nome"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ResumoEquipe struct {
	EquipeID               uuid.UUID           `json:"equipe_id"`
	NomeEquipe             string              `json:"nome_equipe"`
	Periodo                string              `json:"periodo"`
	AtuacaoRastreada       float64             `json:"atuacao_rastreada"`
	TotalHorasEstimadas    float64             `json:"total_horas_estimadas"`
	PercentualMetas        float64             `json:"percentual_metas"`
	PercentualCompromissos float64             `json:"percentual_compromissos"`
	PercentualIniciativas  float64             `json:"percentual_iniciativas"`
	DetalhesIniciativas    DetalhesIniciativas  `json:"detalhes_iniciativas"`
	Membros                []MembroResumo       `json:"membros"`
}

type DetalhesIniciativas struct {
	PercentualManutencao float64 `json:"percentual_manutencao"`
	PercentualMelhorias  float64 `json:"percentual_melhorias"`
	PercentualSuporte    float64 `json:"percentual_suporte"`
}

type MembroResumo struct {
	ID               uuid.UUID `json:"id"`
	Nome             string    `json:"nome"`
	Email            *string   `json:"email"`
	AvatarURL        *string   `json:"avatar_url"`
	AtuacaoRastreada float64   `json:"atuacao_rastreada"`
}

type HorasTarefasMembro struct {
	MembroID                uuid.UUID
	TotalSegundos           int64
	SegundosMetas           int64
	SegundosCompromissos    int64
	SegundosIniciativas     int64
	SegundosManutencao      int64
	SegundosMelhorias       int64
	SegundosSuporte         int64
	SegundosEstimadoAbertos int64
}
