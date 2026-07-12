package domain

import "github.com/google/uuid"

type ResumoEquipe struct {
	NomeEquipe             string              `json:"nome_equipe"`
	Periodo                string              `json:"periodo"`
	AtuacaoRastreada       float64             `json:"atuacao_rastreada"`
	PercentualMetas        float64             `json:"percentual_metas"`
	PercentualCompromissos float64             `json:"percentual_compromissos"`
	PercentualIniciativas  float64             `json:"percentual_iniciativas"`
	DetalhesIniciativas    DetalhesIniciativas  `json:"detalhes_iniciativas"`
	Membros                []MembroResumo       `json:"membros"`
}

type DetalhesIniciativas struct {
	PercentualManutencao    float64 `json:"percentual_manutencao"`
	PercentualMelhorias     float64 `json:"percentual_melhorias"`
	PercentualNovasFeatures float64 `json:"percentual_novas_features"`
	PercentualSuporte       float64 `json:"percentual_suporte"`
}

type MembroResumo struct {
	ID               uuid.UUID `json:"id"`
	Nome             string    `json:"nome"`
	Email            *string   `json:"email"`
	AvatarURL        *string   `json:"avatar_url"`
	AtuacaoRastreada float64   `json:"atuacao_rastreada"`
}

type HorasTarefasMembro struct {
	MembroID              uuid.UUID
	TotalSegundos         int64
	SegundosMetas         int64
	SegundosCompromissos  int64
	SegundosIniciativas   int64
	SegundosManutencao    int64
	SegundosMelhorias     int64
	SegundosNovasFeatures int64
	SegundosSuporte       int64
}
