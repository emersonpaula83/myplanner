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

// EsforcoProjeto é um épico — o que este sistema chama de projeto. Chave é o
// número do ticket (CLOUD-682) e Nome é o apelido, com o resumo como fallback
// porque hoje nenhum épico tem apelido preenchido.
type EsforcoProjeto struct {
	ProjetoID uuid.UUID `json:"projeto_id"`
	Chave     string    `json:"chave"`
	Nome      string    `json:"nome"`
	Cards     int       `json:"cards"`
	Horas     float64   `json:"horas"`
}

// EsforcoCobertura diz sobre quanto do relatório o Top 5 foi calculado. A
// maioria dos cards não tem épico, então o ranking vive num subconjunto e
// somar as cinco linhas não fecha com os números grandes.
type EsforcoCobertura struct {
	Projetos int     `json:"projetos"`
	Cards    int     `json:"cards"`
	Horas    float64 `json:"horas"`
}

// esforcoTopProdutos limita quantos produtos vão no JSON por balde. Há ~80
// produtos ativos; o gráfico lateral mostra os maiores e agrega o resto numa
// linha "Outros", calculada a partir dos totais de EsforcoProdutos.
const esforcoTopProdutos = 8

// EsforcoProduto é um componente do JIRA (tabela produtos). Um card pode ter
// vários, e as horas entram cheias em cada um — decisão de produto para
// responder "quanto esse produto consumiu". Consequência: somar os produtos
// passa das horas do balde. Ver EsforcoProdutos.HorasSomadas.
type EsforcoProduto struct {
	ProdutoID uuid.UUID `json:"produto_id"`
	Nome      string    `json:"nome"`
	Cards     int       `json:"cards"`
	Horas     float64   `json:"horas"`
}

type EsforcoContagem struct {
	Cards int     `json:"cards"`
	Horas float64 `json:"horas"`
}

// EsforcoProdutos é o recorte por produto de um balde da pizza. Produtos traz
// só o topo; TotalProdutos e HorasSomadas cobrem todos, para o frontend montar
// a linha "Outros" e o aviso de dupla contagem sem uma segunda chamada.
//
// Não há total de cards aqui de propósito: somar os cards dos produtos conta
// duas vezes o card que tem dois produtos, e um campo chamado "cards" com esse
// valor mentiria.
type EsforcoProdutos struct {
	Produtos      []EsforcoProduto `json:"produtos"`
	TotalProdutos int              `json:"total_produtos"`
	HorasSomadas  float64          `json:"horas_somadas"`
	SemProduto    EsforcoContagem  `json:"sem_produto"`
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
	Ano                  int               `json:"ano"`
	Trimestres           []int             `json:"trimestres"`
	PeriodoInicio        string            `json:"periodo_inicio"`
	PeriodoFim           string            `json:"periodo_fim"`
	TotalCards           int               `json:"total_cards"`
	TotalHoras           float64           `json:"total_horas"`
	Buckets              []EsforcoBucket   `json:"buckets"`
	TopProjetos          []EsforcoProjeto  `json:"top_projetos"`
	TopProjetosCobertura EsforcoCobertura  `json:"top_projetos_cobertura"`
	Pessoas              []EsforcoPessoa   `json:"pessoas"`
	ForaDoEscopo         EsforcoForaEscopo `json:"fora_do_escopo"`

	// ProdutosPorBucket é o drill-down da pizza: clicar numa fatia abre o
	// recorte por produto daquele balde. Vem junto do relatório porque são só
	// três baldes — não vale um endpoint separado e o clique fica instantâneo.
	// Chave = chave do balde (melhorias|manutencao|outros); os três sempre vêm.
	ProdutosPorBucket map[string]EsforcoProdutos `json:"produtos_por_bucket"`
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
// RECURSIVE está no topo porque topProjetos anexa um CTE recursivo depois de
// `escopo`; as demais consultas não recursam e não são afetadas pela palavra.
const cteBase = `
WITH RECURSIVE periodo AS (
    SELECT make_date($1::int, (q - 1) * 3 + 1, 1) AS ini,
           (make_date($1::int, (q - 1) * 3 + 1, 1) + INTERVAL '3 months')::date AS fim
    FROM unnest($2::int[]) AS q
),
base AS (
    SELECT t.id,
           t.projeto_id,
           t.parent_id,
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

	if rel.TopProjetos, rel.TopProjetosCobertura, err = r.topProjetos(ctx, f, equipeIDs); err != nil {
		return nil, err
	}
	if rel.Pessoas, err = r.pessoas(ctx, f, equipeIDs); err != nil {
		return nil, err
	}
	if rel.ForaDoEscopo, err = r.foraDoEscopo(ctx, f, equipeIDs); err != nil {
		return nil, err
	}
	if rel.ProdutosPorBucket, err = r.produtosPorBucket(ctx, f, equipeIDs); err != nil {
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

// topProjetos ranqueia por PROJETO no sentido do sistema: tarefa de tipo
// 'Épico' (é o que GET /projetos lista, ver repository/timeline.go ListarEpicos).
// A tabela `projetos` guarda o projeto do JIRA, que espelha a equipe — ranquear
// por ela devolveria a lista de times.
//
// Cada card sobe por parent_id até achar um épico. A subida para no primeiro
// épico e o teto de 5 níveis protege contra ciclo, então um card gera no
// máximo uma linha: não há como contar horas em dobro.
//
// Devolve também a cobertura do ranking, porque a maioria dos cards não tem
// pai nenhum e a soma do Top 5 não fecha com o total do relatório.
func (r *RelatorioEsforcoRepository) topProjetos(ctx context.Context, f RelatorioEsforcoFiltro, equipeIDs []uuid.UUID) ([]EsforcoProjeto, EsforcoCobertura, error) {
	var cob EsforcoCobertura

	rows, err := r.pool.Query(ctx, cteBase+`
		, sobe(tarefa_id, horas, atual, nivel) AS (
			SELECT e.id, e.horas, e.parent_id, 1
			FROM escopo e
			WHERE e.parent_id IS NOT NULL
			UNION ALL
			SELECT s.tarefa_id, s.horas, t.parent_id, s.nivel + 1
			FROM sobe s
			JOIN tarefas t ON t.id = s.atual
			WHERE t.tipo <> 'Épico'
			  AND t.parent_id IS NOT NULL
			  AND s.nivel < 5
		),
		agrupado AS (
			SELECT ep.id,
			       ep.numero_ticket,
			       COALESCE(NULLIF(ep.apelido, ''), ep.resumo) AS nome,
			       count(*) AS cards,
			       COALESCE(sum(s.horas), 0)::float8 AS horas
			FROM sobe s
			JOIN tarefas ep ON ep.id = s.atual AND ep.tipo = 'Épico'
			GROUP BY ep.id, ep.numero_ticket, COALESCE(NULLIF(ep.apelido, ''), ep.resumo)
		)
		SELECT id, numero_ticket, nome, cards, horas,
		       (SELECT count(*) FROM agrupado),
		       (SELECT COALESCE(sum(cards), 0) FROM agrupado),
		       (SELECT COALESCE(sum(horas), 0)::float8 FROM agrupado)
		FROM agrupado
		ORDER BY horas DESC, nome
		LIMIT 5
	`, f.Ano, f.Trimestres, equipeIDs)
	if err != nil {
		return nil, cob, fmt.Errorf("agregando top projetos: %w", err)
	}
	defer rows.Close()

	projetos := []EsforcoProjeto{}
	for rows.Next() {
		var p EsforcoProjeto
		if err := rows.Scan(&p.ProjetoID, &p.Chave, &p.Nome, &p.Cards, &p.Horas,
			&cob.Projetos, &cob.Cards, &cob.Horas); err != nil {
			return nil, cob, fmt.Errorf("lendo top projeto: %w", err)
		}
		projetos = append(projetos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, cob, fmt.Errorf("iterando top projetos: %w", err)
	}

	return projetos, cob, nil
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

// produtoLinha é uma linha crua da consulta de produtos. ProdutoID nulo é o
// card que não tem nenhum produto vinculado — vem do LEFT JOIN.
type produtoLinha struct {
	Bucket    string
	ProdutoID *uuid.UUID
	Nome      string
	Cards     int
	Horas     float64
}

// produtosPorBucket quebra cada balde da pizza por produto (JIRA Component).
//
// O LEFT JOIN traz numa passada só os cards com produto e os sem: card com N
// produtos gera N linhas e entra com as horas cheias em cada uma, card sem
// produto gera uma linha com produto_id nulo. Por isso a soma dos produtos
// passa das horas do balde (~11% no histórico) — o frontend avisa disso.
func (r *RelatorioEsforcoRepository) produtosPorBucket(ctx context.Context, f RelatorioEsforcoFiltro, equipeIDs []uuid.UUID) (map[string]EsforcoProdutos, error) {
	rows, err := r.pool.Query(ctx, cteBase+`
		SELECT e.bucket, p.id, COALESCE(p.nome, ''),
		       count(*), COALESCE(sum(e.horas), 0)::float8
		FROM escopo e
		LEFT JOIN tarefa_produtos tp ON tp.tarefa_id = e.id
		LEFT JOIN produtos p ON p.id = tp.produto_id
		GROUP BY e.bucket, p.id, p.nome
	`, f.Ano, f.Trimestres, equipeIDs)
	if err != nil {
		return nil, fmt.Errorf("agregando esforço por produto: %w", err)
	}
	defer rows.Close()

	linhas := []produtoLinha{}
	for rows.Next() {
		var l produtoLinha
		if err := rows.Scan(&l.Bucket, &l.ProdutoID, &l.Nome, &l.Cards, &l.Horas); err != nil {
			return nil, fmt.Errorf("lendo esforço por produto: %w", err)
		}
		linhas = append(linhas, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando esforço por produto: %w", err)
	}

	return montarProdutosPorBucket(linhas), nil
}

// montarProdutosPorBucket agrupa as linhas cruas por balde, ordena por horas e
// corta no topo, guardando os totais completos. Sempre devolve os três baldes:
// o frontend indexa pela chave da fatia clicada.
func montarProdutosPorBucket(linhas []produtoLinha) map[string]EsforcoProdutos {
	porBucket := map[string]EsforcoProdutos{}
	for _, def := range bucketsOrdenados {
		porBucket[def.Chave] = EsforcoProdutos{Produtos: []EsforcoProduto{}}
	}

	for _, l := range linhas {
		b, ok := porBucket[l.Bucket]
		if !ok {
			continue
		}
		if l.ProdutoID == nil {
			b.SemProduto.Cards += l.Cards
			b.SemProduto.Horas += l.Horas
			porBucket[l.Bucket] = b
			continue
		}
		b.Produtos = append(b.Produtos, EsforcoProduto{
			ProdutoID: *l.ProdutoID,
			Nome:      l.Nome,
			Cards:     l.Cards,
			Horas:     l.Horas,
		})
		b.TotalProdutos++
		b.HorasSomadas += l.Horas
		porBucket[l.Bucket] = b
	}

	for chave, b := range porBucket {
		sort.Slice(b.Produtos, func(i, j int) bool {
			if b.Produtos[i].Horas != b.Produtos[j].Horas {
				return b.Produtos[i].Horas > b.Produtos[j].Horas
			}
			return b.Produtos[i].Nome < b.Produtos[j].Nome
		})
		if len(b.Produtos) > esforcoTopProdutos {
			b.Produtos = b.Produtos[:esforcoTopProdutos]
		}
		porBucket[chave] = b
	}

	return porBucket
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
