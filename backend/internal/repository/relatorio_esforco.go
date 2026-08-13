package repository

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Chaves e rótulos dos baldes de esforço. A ordem define a ordem das fatias
// da pizza — mantida fixa para que um balde vazio não reordene o gráfico.
const (
	BucketMelhorias  = "melhorias"
	BucketManutencao = "manutencao"
	BucketOutros     = "outros"
)

var bucketsOrdenados = []struct {
	Chave  string
	Rotulo string
}{
	{BucketMelhorias, "Melhorias e Inovações"},
	{BucketManutencao, "Manutenção"},
	{BucketOutros, "Outros"},
}

type RelatorioEsforcoRepository struct {
	pool *pgxpool.Pool
}

func NewRelatorioEsforcoRepository(pool *pgxpool.Pool) *RelatorioEsforcoRepository {
	return &RelatorioEsforcoRepository{pool: pool}
}

type RelatorioEsforcoFiltro struct {
	Ano        int
	Trimestres []int
	EquipeIDs  []uuid.UUID
}

type EsforcoBucket struct {
	Chave      string  `json:"chave"`
	Rotulo     string  `json:"rotulo"`
	Cards      int     `json:"cards"`
	Horas      float64 `json:"horas"`
	Percentual float64 `json:"percentual"`
}

type EsforcoProjeto struct {
	ProjetoID uuid.UUID `json:"projeto_id"`
	Chave     string    `json:"chave"`
	Nome      string    `json:"nome"`
	Cards     int       `json:"cards"`
	Horas     float64   `json:"horas"`
}

type EsforcoPessoa struct {
	MembroID   uuid.UUID       `json:"membro_id"`
	Nome       string          `json:"nome"`
	AvatarURL  *string         `json:"avatar_url"`
	EquipeID   uuid.UUID       `json:"equipe_id"`
	EquipeNome string          `json:"equipe_nome"`
	Cards      int             `json:"cards"`
	Horas      float64         `json:"horas"`
	Buckets    []EsforcoBucket `json:"buckets"`
}

// EsforcoForaEscopo mede o que ficou de fora por falta de cadastro: cards
// concluídos cujo responsável não pertence a nenhuma equipe. Não é afetado
// pelo filtro de equipes — é sempre o buraco total do período.
type EsforcoForaEscopo struct {
	Cards int     `json:"cards"`
	Horas float64 `json:"horas"`
}

type RelatorioEsforco struct {
	Ano           int               `json:"ano"`
	Trimestres    []int             `json:"trimestres"`
	PeriodoInicio string            `json:"periodo_inicio"`
	PeriodoFim    string            `json:"periodo_fim"`
	TotalCards    int               `json:"total_cards"`
	TotalHoras    float64           `json:"total_horas"`
	Buckets       []EsforcoBucket   `json:"buckets"`
	TopProjetos   []EsforcoProjeto  `json:"top_projetos"`
	Pessoas       []EsforcoPessoa   `json:"pessoas"`
	ForaDoEscopo  EsforcoForaEscopo `json:"fora_do_escopo"`
}

// cteBase monta o recorte comum a todas as consultas do relatório:
//
//	$1 ano, $2 trimestres (array), $3 equipe_ids (array vazio = todas as equipes)
//
// Regras travadas no design:
//   - só cards concluídos (status_categoria = 'done') resolvidos nos trimestres
//   - Épico e Projeto ficam de fora: são contêineres e a estimativa deles
//     agrega a dos filhos, o que contaria o mesmo trabalho duas vezes
//   - horas vêm de estimativa_tempo (cobre 90% dos cards; tempo_gasto cobre 10%)
//
// O JOIN com periodo não duplica card: trimestres são intervalos disjuntos,
// então cada data_resolvido casa com no máximo uma linha de periodo.
const cteBase = `
WITH periodo AS (
    SELECT make_date($1::int, (q - 1) * 3 + 1, 1) AS ini,
           (make_date($1::int, (q - 1) * 3 + 1, 1) + INTERVAL '3 months')::date AS fim
    FROM unnest($2::int[]) AS q
),
base AS (
    SELECT t.id,
           t.projeto_id,
           t.responsavel_id,
           COALESCE(t.estimativa_tempo, 0)::numeric / 3600 AS horas,
           CASE
               WHEN t.tipo IN ('História', 'Melhoria') THEN 'melhorias'
               WHEN t.tipo IN ('Bug', '[System] Incidente') THEN 'manutencao'
               ELSE 'outros'
           END AS bucket
    FROM tarefas t
    JOIN periodo p ON t.data_resolvido >= p.ini AND t.data_resolvido < p.fim
    WHERE t.removido_em IS NULL
      AND t.status_categoria = 'done'
      AND t.tipo NOT IN ('Épico', 'Projeto')
),
escopo AS (
    SELECT b.*, em.equipe_id
    FROM base b
    JOIN equipe_membros em ON em.membro_id = b.responsavel_id
    WHERE cardinality($3::uuid[]) = 0 OR em.equipe_id = ANY($3::uuid[])
)
`

func (r *RelatorioEsforcoRepository) Get(ctx context.Context, f RelatorioEsforcoFiltro) (*RelatorioEsforco, error) {
	equipeIDs := f.EquipeIDs
	if equipeIDs == nil {
		equipeIDs = []uuid.UUID{}
	}

	rel := &RelatorioEsforco{
		Ano:        f.Ano,
		Trimestres: f.Trimestres,
	}

	// Período mostrado é a borda externa do conjunto: menor início, maior fim.
	if err := r.pool.QueryRow(ctx, `
		SELECT to_char(min(make_date($1::int, (q - 1) * 3 + 1, 1)), 'YYYY-MM-DD'),
		       to_char(max((make_date($1::int, (q - 1) * 3 + 1, 1) + INTERVAL '3 months' - INTERVAL '1 day')::date), 'YYYY-MM-DD')
		FROM unnest($2::int[]) AS q
	`, f.Ano, f.Trimestres).Scan(&rel.PeriodoInicio, &rel.PeriodoFim); err != nil {
		return nil, fmt.Errorf("calculando período dos trimestres: %w", err)
	}

	buckets, err := r.buckets(ctx, f, equipeIDs)
	if err != nil {
		return nil, err
	}
	rel.Buckets = buckets
	for _, b := range buckets {
		rel.TotalCards += b.Cards
		rel.TotalHoras += b.Horas
	}
	aplicarPercentuais(rel.Buckets, rel.TotalHoras)

	if rel.TopProjetos, err = r.topProjetos(ctx, f, equipeIDs); err != nil {
		return nil, err
	}
	if rel.Pessoas, err = r.pessoas(ctx, f, equipeIDs); err != nil {
		return nil, err
	}
	if rel.ForaDoEscopo, err = r.foraDoEscopo(ctx, f, equipeIDs); err != nil {
		return nil, err
	}

	return rel, nil
}

func (r *RelatorioEsforcoRepository) buckets(ctx context.Context, f RelatorioEsforcoFiltro, equipeIDs []uuid.UUID) ([]EsforcoBucket, error) {
	rows, err := r.pool.Query(ctx, cteBase+`
		SELECT bucket, count(*), COALESCE(sum(horas), 0)::float8
		FROM escopo
		GROUP BY bucket
	`, f.Ano, f.Trimestres, equipeIDs)
	if err != nil {
		return nil, fmt.Errorf("agregando baldes de esforço: %w", err)
	}
	defer rows.Close()

	porChave := map[string]EsforcoBucket{}
	for rows.Next() {
		var chave string
		var cards int
		var horas float64
		if err := rows.Scan(&chave, &cards, &horas); err != nil {
			return nil, fmt.Errorf("lendo balde de esforço: %w", err)
		}
		porChave[chave] = EsforcoBucket{Cards: cards, Horas: horas}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando baldes de esforço: %w", err)
	}

	return montarBuckets(porChave), nil
}

func (r *RelatorioEsforcoRepository) topProjetos(ctx context.Context, f RelatorioEsforcoFiltro, equipeIDs []uuid.UUID) ([]EsforcoProjeto, error) {
	rows, err := r.pool.Query(ctx, cteBase+`
		SELECT p.id, p.chave, p.nome, count(*), COALESCE(sum(e.horas), 0)::float8 AS horas
		FROM escopo e
		JOIN projetos p ON p.id = e.projeto_id
		GROUP BY p.id, p.chave, p.nome
		ORDER BY horas DESC
		LIMIT 5
	`, f.Ano, f.Trimestres, equipeIDs)
	if err != nil {
		return nil, fmt.Errorf("agregando top projetos: %w", err)
	}
	defer rows.Close()

	projetos := []EsforcoProjeto{}
	for rows.Next() {
		var p EsforcoProjeto
		if err := rows.Scan(&p.ProjetoID, &p.Chave, &p.Nome, &p.Cards, &p.Horas); err != nil {
			return nil, fmt.Errorf("lendo top projeto: %w", err)
		}
		projetos = append(projetos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando top projetos: %w", err)
	}

	return projetos, nil
}

func (r *RelatorioEsforcoRepository) pessoas(ctx context.Context, f RelatorioEsforcoFiltro, equipeIDs []uuid.UUID) ([]EsforcoPessoa, error) {
	rows, err := r.pool.Query(ctx, cteBase+`
		SELECT m.id, m.nome, m.avatar_url, eq.id, eq.nome, e.bucket,
		       count(*), COALESCE(sum(e.horas), 0)::float8
		FROM escopo e
		JOIN membros m ON m.id = e.responsavel_id
		JOIN equipes eq ON eq.id = e.equipe_id
		GROUP BY m.id, m.nome, m.avatar_url, eq.id, eq.nome, e.bucket
	`, f.Ano, f.Trimestres, equipeIDs)
	if err != nil {
		return nil, fmt.Errorf("agregando esforço por pessoa: %w", err)
	}
	defer rows.Close()

	type acumulado struct {
		pessoa EsforcoPessoa
		baldes map[string]EsforcoBucket
	}
	porMembro := map[uuid.UUID]*acumulado{}
	ordem := []uuid.UUID{}

	for rows.Next() {
		var membroID, equipeID uuid.UUID
		var nome, equipeNome, bucket string
		var avatar *string
		var cards int
		var horas float64
		if err := rows.Scan(&membroID, &nome, &avatar, &equipeID, &equipeNome, &bucket, &cards, &horas); err != nil {
			return nil, fmt.Errorf("lendo esforço por pessoa: %w", err)
		}

		acc, ok := porMembro[membroID]
		if !ok {
			acc = &acumulado{
				pessoa: EsforcoPessoa{
					MembroID:   membroID,
					Nome:       nome,
					AvatarURL:  avatar,
					EquipeID:   equipeID,
					EquipeNome: equipeNome,
				},
				baldes: map[string]EsforcoBucket{},
			}
			porMembro[membroID] = acc
			ordem = append(ordem, membroID)
		}
		acc.baldes[bucket] = EsforcoBucket{Cards: cards, Horas: horas}
		acc.pessoa.Cards += cards
		acc.pessoa.Horas += horas
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando esforço por pessoa: %w", err)
	}

	pessoas := make([]EsforcoPessoa, 0, len(ordem))
	for _, id := range ordem {
		acc := porMembro[id]
		acc.pessoa.Buckets = montarBuckets(acc.baldes)
		aplicarPercentuais(acc.pessoa.Buckets, acc.pessoa.Horas)
		pessoas = append(pessoas, acc.pessoa)
	}

	ordenarPessoasPorHoras(pessoas)
	return pessoas, nil
}

func (r *RelatorioEsforcoRepository) foraDoEscopo(ctx context.Context, f RelatorioEsforcoFiltro, equipeIDs []uuid.UUID) (EsforcoForaEscopo, error) {
	var fora EsforcoForaEscopo
	err := r.pool.QueryRow(ctx, cteBase+`
		SELECT count(*), COALESCE(sum(b.horas), 0)::float8
		FROM base b
		WHERE NOT EXISTS (
			SELECT 1 FROM equipe_membros em WHERE em.membro_id = b.responsavel_id
		)
	`, f.Ano, f.Trimestres, equipeIDs).Scan(&fora.Cards, &fora.Horas)
	if err != nil {
		return fora, fmt.Errorf("medindo cards fora do escopo: %w", err)
	}
	return fora, nil
}

// montarBuckets devolve sempre os três baldes na ordem fixa, preenchendo com
// zero os que não apareceram na consulta.
func montarBuckets(porChave map[string]EsforcoBucket) []EsforcoBucket {
	out := make([]EsforcoBucket, 0, len(bucketsOrdenados))
	for _, def := range bucketsOrdenados {
		b := porChave[def.Chave]
		b.Chave = def.Chave
		b.Rotulo = def.Rotulo
		out = append(out, b)
	}
	return out
}

func aplicarPercentuais(buckets []EsforcoBucket, total float64) {
	if total <= 0 {
		return
	}
	for i := range buckets {
		buckets[i].Percentual = buckets[i].Horas / total * 100
	}
}

func ordenarPessoasPorHoras(pessoas []EsforcoPessoa) {
	sort.Slice(pessoas, func(i, j int) bool {
		if pessoas[i].Horas != pessoas[j].Horas {
			return pessoas[i].Horas > pessoas[j].Horas
		}
		return pessoas[i].Nome < pessoas[j].Nome
	})
}
