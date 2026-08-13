package domain

import (
	"time"

	"github.com/google/uuid"
)

// MembroFusaoLado é um dos dois registros da tela de comparação. Além dos
// campos de RH que o operador escolhe, traz contagens para ele enxergar o
// tamanho da operação antes de confirmar.
type MembroFusaoLado struct {
	ID               uuid.UUID  `json:"id"`
	Nome             string     `json:"nome"`
	Email            *string    `json:"email"`
	JiraAccountID    string     `json:"jira_account_id"`
	Cargo            *string    `json:"cargo"`
	Salario          *float64   `json:"salario"`
	Matricula        *string    `json:"matricula"`
	DataAdmissao     *time.Time `json:"data_admissao"`
	UltimoAumento    *time.Time `json:"ultimo_aumento"`
	GestorID         *uuid.UUID `json:"gestor_id"`
	GestorNome       *string    `json:"gestor_nome"`
	Tarefas          int        `json:"tarefas"`
	EquipeNome       *string    `json:"equipe_nome"`
	RegistrosSalario int        `json:"registros_salario"`
	Skills           int        `json:"skills"`
	Ausencias        int        `json:"ausencias"`
}

// MembroFusaoPreview alimenta a tela de comparação. SobreviventeSugeridoID é o
// lado com mais tarefas — é só sugestão, o operador pode inverter.
type MembroFusaoPreview struct {
	Membro                 MembroFusaoLado `json:"membro"`
	Alvo                   MembroFusaoLado `json:"alvo"`
	SobreviventeSugeridoID uuid.UUID       `json:"sobrevivente_sugerido_id"`
}

// MembroFusaoCampos são os valores finais escolhidos pelo operador. Campo nil
// significa "não alterar o que o sobrevivente já tem"; não há como limpar um
// campo pela fusão. Datas em "2006-01-02".
type MembroFusaoCampos struct {
	Cargo         *string    `json:"cargo"`
	Salario       *float64   `json:"salario"`
	Matricula     *string    `json:"matricula"`
	DataAdmissao  *string    `json:"data_admissao"`
	UltimoAumento *string    `json:"ultimo_aumento"`
	GestorID      *uuid.UUID `json:"gestor_id"`
}

type MembroFusaoRequest struct {
	MembroPerdedorID uuid.UUID         `json:"membro_perdedor_id"`
	Campos           MembroFusaoCampos `json:"campos"`
}

// MembroFusaoResultado é o que a tela mostra depois de confirmar.
type MembroFusaoResultado struct {
	TarefasRepontadas     int  `json:"tarefas_repontadas"`
	RegistrosHistorico    int  `json:"registros_historico"`
	VinculoAtivoEncerrado bool `json:"vinculo_ativo_encerrado"`
	ContasAbsorvidas      int  `json:"contas_absorvidas"`
}
