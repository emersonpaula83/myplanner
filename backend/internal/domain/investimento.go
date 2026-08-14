package domain

import (
	"time"

	"github.com/google/uuid"
)

type SalarioHistorico struct {
	ID           uuid.UUID `json:"id"`
	MembroID     uuid.UUID `json:"membro_id"`
	Valor        *float64  `json:"valor,omitempty"`
	DataVigencia time.Time `json:"data_vigencia"`
	CreatedAt    time.Time `json:"created_at"`
}

type BancoHorasHistorico struct {
	ID           uuid.UUID `json:"id"`
	MembroID     uuid.UUID `json:"membro_id"`
	Valor        float64   `json:"valor"`
	DataRegistro time.Time `json:"data_registro"`
	CreatedAt    time.Time `json:"created_at"`
}

type InvestimentoDashboard struct {
	Equipe  EquipeInfo           `json:"equipe"`
	Sumario InvestimentoSumario  `json:"sumario"`
	Membros []MembroInvestimento `json:"membros"`
}

type EquipeInfo struct {
	ID   uuid.UUID `json:"id"`
	Nome string    `json:"nome"`
}

type InvestimentoSumario struct {
	// Ponteiro com omitempty: travado, a chave some do JSON. Zerar seria mentir
	// — "R$ 0,00" é um valor, e o frontend não distinguiria de custo real zero.
	CustoMensalTotal    *float64 `json:"custo_mensal_total,omitempty"`
	TotalMembros        int      `json:"total_membros"`
	TempoCasaMedioMeses int      `json:"tempo_casa_medio_meses"`
	BancoHorasTotal     float64  `json:"banco_horas_total"`
}

type MembroInvestimento struct {
	ID        uuid.UUID `json:"id"`
	Nome      string    `json:"nome"`
	AvatarURL *string   `json:"avatar_url"`
	// omitempty: travado, o handler zera este ponteiro e a chave precisa sumir
	// do JSON — "salario":null ainda vaza a existência do campo no F12.
	Salario        *float64 `json:"salario,omitempty"`
	DataAdmissao   *string  `json:"data_admissao"`
	TempoCasaMeses int      `json:"tempo_casa_meses"`
	BancoHoras     *float64 `json:"banco_horas"`
	Cargo          *string  `json:"cargo"`
	TopProdutos    []string `json:"top_produtos"`
}

type GastoMensal struct {
	Mes        int      `json:"mes"`
	CustoTotal *float64 `json:"custo_total,omitempty"`
}

type GastosMensaisResponse struct {
	Ano   int           `json:"ano"`
	Meses []GastoMensal `json:"meses"`
}

type ProjetoAlocacao struct {
	Apelido            string  `json:"apelido"`
	ChaveJira          string  `json:"chave_jira"`
	PercentualAlocacao float64 `json:"percentual_alocacao"`
}

type AlocacoesProjetosResponse struct {
	Projetos []ProjetoAlocacao `json:"projetos"`
}
