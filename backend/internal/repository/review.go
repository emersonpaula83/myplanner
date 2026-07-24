package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReviewRepository struct {
	pool *pgxpool.Pool
}

func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{pool: pool}
}

func (r *ReviewRepository) Pool() *pgxpool.Pool {
	return r.pool
}

type ReviewTaskRow struct {
	ID           uuid.UUID   `json:"id"`
	NumeroTicket string      `json:"numero_ticket"`
	Resumo       string      `json:"resumo"`
	Tipo         string      `json:"tipo"`
	Status       string      `json:"status"`
	ParentID     *uuid.UUID  `json:"parent_id"`
	RelatorNome  *string     `json:"relator_nome"`
	NaoPlanejada bool        `json:"nao_planejada"`
	Produtos     []string    `json:"produtos"`
	ProdutoIDs   []uuid.UUID `json:"produto_ids"`
}

type ReviewPO struct {
	Nome     string   `json:"nome"`
	Produtos []string `json:"produtos"`
}

type ReviewDestaque struct {
	ID           uuid.UUID `json:"id"`
	SprintID     uuid.UUID `json:"sprint_id"`
	EquipeID     uuid.UUID `json:"equipe_id"`
	ProdutoID    uuid.UUID `json:"produto_id"`
	ProdutoNome  string    `json:"produto_nome"`
	Titulo       string    `json:"titulo"`
	Descricao    string    `json:"descricao"`
	Link         *string   `json:"link"`
	Ordem        int       `json:"ordem"`
	CriadoEm     time.Time `json:"criado_em"`
	AtualizadoEm time.Time `json:"atualizado_em"`
}

func (r *ReviewRepository) GetReviewTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]ReviewTaskRow, error) {
	argN := 1
	args := []interface{}{sprintID}

	equipeJoin := ""
	equipeWhere := ""
	if equipeID != nil {
		argN++
		args = append(args, *equipeID)
		equipeJoin = "INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id"
		equipeWhere = fmt.Sprintf("AND em.equipe_id = $%d", argN)
	}

	produtoJoin := ""
	produtoWhere := ""
	if len(produtoIDs) > 0 {
		placeholders := make([]string, len(produtoIDs))
		for i, pid := range produtoIDs {
			argN++
			args = append(args, pid)
			placeholders[i] = fmt.Sprintf("$%d", argN)
		}
		produtoJoin = "INNER JOIN tarefa_produtos tp_filter ON tp_filter.tarefa_id = t.id"
		produtoWhere = fmt.Sprintf("AND tp_filter.produto_id IN (%s)", strings.Join(placeholders, ","))
	}

	query := fmt.Sprintf(`
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status,
		       t.parent_id, m.nome,
		       CASE WHEN t.data_entrada_sprint > s.data_inicio
		            OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)
		            THEN true ELSE false END AS nao_planejada,
		       ARRAY_AGG(p.nome ORDER BY p.id) FILTER (WHERE p.nome IS NOT NULL) AS produtos,
		       ARRAY_AGG(p.id ORDER BY p.id) FILTER (WHERE p.id IS NOT NULL) AS produto_ids
		FROM tarefas t
		INNER JOIN sprints s ON s.id = t.sprint_id
		LEFT JOIN membros m ON m.id = t.relator_id
		LEFT JOIN tarefa_produtos tp ON tp.tarefa_id = t.id
		LEFT JOIN produtos p ON p.id = tp.produto_id
		%s
		%s
		WHERE t.sprint_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		  %s %s
		GROUP BY t.id, t.numero_ticket, t.resumo, t.tipo, t.status,
		         t.parent_id, m.nome, t.data_entrada_sprint, t.data_criacao,
		         s.data_inicio
		ORDER BY t.numero_ticket
	`, equipeJoin, produtoJoin, equipeWhere, produtoWhere)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying review tasks: %w", err)
	}
	defer rows.Close()

	result := make([]ReviewTaskRow, 0)
	for rows.Next() {
		var row ReviewTaskRow
		if err := rows.Scan(
			&row.ID, &row.NumeroTicket, &row.Resumo, &row.Tipo, &row.Status,
			&row.ParentID, &row.RelatorNome, &row.NaoPlanejada, &row.Produtos, &row.ProdutoIDs,
		); err != nil {
			return nil, fmt.Errorf("scanning review task: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *ReviewRepository) GetGDPTCAncestorTaskIDs(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT t.id AS original_id, t.id, t.parent_id, t.numero_ticket
			FROM tarefas t WHERE t.id = ANY($1)
			UNION ALL
			SELECT a.original_id, p.id, p.parent_id, p.numero_ticket
			FROM tarefas p JOIN ancestors a ON p.id = a.parent_id
		)
		SELECT DISTINCT original_id FROM ancestors
		WHERE numero_ticket LIKE 'GDPTC-%' AND original_id != id
	`, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("querying GDPTC ancestors: %w", err)
	}
	defer rows.Close()

	result := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning GDPTC ancestor: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (r *ReviewRepository) GetReviewPOs(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]ReviewPO, error) {
	args := []interface{}{equipeID}
	produtoFilter := ""
	if len(produtoIDs) > 0 {
		args = append(args, produtoIDs)
		produtoFilter = "AND p.id = ANY($2)"
	}

	// NOTE: membros has no equipe_id column directly; team membership is
	// resolved via the equipe_membros junction table.
	query := fmt.Sprintf(`
		SELECT m.nome, ARRAY_AGG(DISTINCT p.nome) AS produtos
		FROM membros m
		JOIN equipe_membros em ON em.membro_id = m.id
		JOIN membro_produtos mp ON mp.membro_id = m.id
		JOIN produtos p ON p.id = mp.produto_id
		WHERE em.equipe_id = $1 AND m.cargo = 'po_produto'
		  %s
		GROUP BY m.id, m.nome
		ORDER BY m.nome
	`, produtoFilter)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying review POs: %w", err)
	}
	defer rows.Close()

	result := make([]ReviewPO, 0)
	for rows.Next() {
		var po ReviewPO
		if err := rows.Scan(&po.Nome, &po.Produtos); err != nil {
			return nil, fmt.Errorf("scanning review PO: %w", err)
		}
		result = append(result, po)
	}
	return result, rows.Err()
}

func (r *ReviewRepository) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]ReviewDestaque, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.sprint_id, d.equipe_id, d.produto_id, p.nome,
		       d.titulo, d.descricao, d.link, d.ordem,
		       d.criado_em, d.atualizado_em
		FROM sprint_review_destaques d
		JOIN produtos p ON p.id = d.produto_id
		WHERE d.sprint_id = $1 AND d.equipe_id = $2
		ORDER BY p.nome, d.ordem
	`, sprintID, equipeID)
	if err != nil {
		return nil, fmt.Errorf("listing destaques: %w", err)
	}
	defer rows.Close()

	result := make([]ReviewDestaque, 0)
	for rows.Next() {
		var d ReviewDestaque
		if err := rows.Scan(
			&d.ID, &d.SprintID, &d.EquipeID, &d.ProdutoID, &d.ProdutoNome,
			&d.Titulo, &d.Descricao, &d.Link, &d.Ordem,
			&d.CriadoEm, &d.AtualizadoEm,
		); err != nil {
			return nil, fmt.Errorf("scanning destaque: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *ReviewRepository) CreateDestaque(ctx context.Context, d ReviewDestaque) (ReviewDestaque, error) {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO sprint_review_destaques (sprint_id, equipe_id, produto_id, titulo, descricao, link, ordem)
		VALUES ($1, $2, $3, $4, $5, $6,
			COALESCE((SELECT MAX(ordem) + 1 FROM sprint_review_destaques
			          WHERE sprint_id = $1 AND equipe_id = $2 AND produto_id = $3), 0))
		RETURNING id, ordem, criado_em, atualizado_em
	`, d.SprintID, d.EquipeID, d.ProdutoID, d.Titulo, d.Descricao, d.Link,
	).Scan(&d.ID, &d.Ordem, &d.CriadoEm, &d.AtualizadoEm)
	if err != nil {
		return ReviewDestaque{}, fmt.Errorf("creating destaque: %w", err)
	}
	return d, nil
}

func (r *ReviewRepository) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (ReviewDestaque, error) {
	var d ReviewDestaque
	err := r.pool.QueryRow(ctx, `
		UPDATE sprint_review_destaques
		SET titulo = $2, descricao = $3, link = $4, atualizado_em = NOW()
		WHERE id = $1
		RETURNING id, sprint_id, equipe_id, produto_id, titulo, descricao, link, ordem, criado_em, atualizado_em
	`, id, titulo, descricao, link).Scan(
		&d.ID, &d.SprintID, &d.EquipeID, &d.ProdutoID,
		&d.Titulo, &d.Descricao, &d.Link, &d.Ordem,
		&d.CriadoEm, &d.AtualizadoEm,
	)
	if err != nil {
		return ReviewDestaque{}, fmt.Errorf("updating destaque: %w", err)
	}
	return d, nil
}

func (r *ReviewRepository) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM sprint_review_destaques WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting destaque: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
