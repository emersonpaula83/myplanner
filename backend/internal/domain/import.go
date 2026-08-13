package domain

import "github.com/google/uuid"

type ImportPlanilhaLinha struct {
	Linha         int
	Nome          string
	Gestao        string
	TimeSquad     string
	Funcao        string
	Matricula     *string
	Admissao      *string
	Salario       *float64
	UltimoAumento *string
}

type ImportIgnorado struct {
	Linha  int    `json:"linha"`
	Nome   string `json:"nome"`
	Motivo string `json:"motivo"`
}

type ImportParseResult struct {
	Linhas    []ImportPlanilhaLinha
	Ignorados []ImportIgnorado
}

type ImportDados struct {
	Cargo         *string    `json:"cargo"`
	Matricula     *string    `json:"matricula"`
	Salario       *float64   `json:"salario"`
	DataAdmissao  *string    `json:"data_admissao"`
	UltimoAumento *string    `json:"ultimo_aumento"`
	GestorNome    string     `json:"gestor_nome"`
	GestorID      *uuid.UUID `json:"gestor_id"`
}

type ImportMatched struct {
	Linha        int         `json:"linha"`
	NomePlanilha string      `json:"nome_planilha"`
	MembroID     uuid.UUID   `json:"membro_id"`
	MembroNome   string      `json:"membro_nome"`
	EquipeID     *uuid.UUID  `json:"equipe_id"`
	EquipeNome   string      `json:"equipe_nome"`
	Dados        ImportDados `json:"dados"`
	Changes      []string    `json:"changes"`
}

type ImportUnmatchedMembro struct {
	Linha        int         `json:"linha"`
	NomePlanilha string      `json:"nome_planilha"`
	Dados        ImportDados `json:"dados"`
}

type ImportUnmatchedEquipe struct {
	NomePlanilha string `json:"nome_planilha"`
	Linhas       []int  `json:"linhas"`
}

type ImportUnmatchedGestor struct {
	NomePlanilha string `json:"nome_planilha"`
	Linhas       []int  `json:"linhas"`
}

type ImportMatchResult struct {
	Matched           []ImportMatched         `json:"matched"`
	UnmatchedMembros  []ImportUnmatchedMembro `json:"unmatched_membros"`
	UnmatchedEquipes  []ImportUnmatchedEquipe `json:"unmatched_equipes"`
	UnmatchedGestores []ImportUnmatchedGestor `json:"unmatched_gestores"`
	Ignorados         []ImportIgnorado        `json:"ignorados"`
}
