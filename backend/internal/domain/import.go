package domain

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
